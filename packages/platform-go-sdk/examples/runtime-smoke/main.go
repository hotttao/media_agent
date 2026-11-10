// Opt-in live acceptance: stdin supplies application credentials and an explicitly authorized authFile.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sdk "media_agent/packages/platform-go-sdk"
	"os"
	"strconv"
	"strings"
	"time"
)

type input struct{ Base, App, Secret, External, Team, AuthFile string }
type event struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

func consume(ctx context.Context, u *sdk.UserClient, id string, after int64, first bool) ([]event, error) {
	response, err := u.Events(ctx, id, after)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	scan := bufio.NewScanner(response.Body)
	scan.Buffer(make([]byte, 4096), 1<<20)
	events := []event{}
	var seq int64
	data := ""
	for scan.Scan() {
		line := strings.TrimSuffix(scan.Text(), "\r")
		switch {
		case strings.HasPrefix(line, "id:"):
			seq, _ = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "id:")), 10, 64)
		case strings.HasPrefix(line, "data:"):
			data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case line == "":
			if seq > 0 && data != "" {
				var payload struct {
					Type string `json:"type"`
				}
				if json.Unmarshal([]byte(data), &payload) != nil {
					return nil, errors.New("invalid event")
				}
				if seq <= after {
					return nil, errors.New("cursor replayed")
				}
				events = append(events, event{seq, payload.Type})
				if first || payload.Type == "turn.completed" || payload.Type == "turn.failed" || payload.Type == "turn.interrupted" {
					return events, nil
				}
			}
			seq = 0
			data = ""
		}
	}
	if scan.Err() != nil {
		return nil, errors.New("event stream read failed")
	}
	return nil, errors.New("stream ended before terminal event")
}
func run(in input) (report map[string]any, err error) {
	if in.AuthFile == "" {
		return nil, errors.New("authFile must explicitly identify an authorized Codex auth file")
	}
	raw, err := os.ReadFile(in.AuthFile)
	if err != nil {
		return nil, errors.New("cannot read authorized auth file")
	}
	c, err := sdk.New(sdk.Config{BaseURL: in.Base, ClientID: in.App, ClientSecret: in.Secret})
	if err != nil {
		return nil, err
	}
	u, err := c.As(in.External, in.Team, sdk.WorkspaceRead, sdk.WorkspaceEnsure, sdk.SessionChat)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	conn, err := u.CreateModelConnection(ctx, sdk.ModelConnectionInput{Name: "Go SDK OAuth acceptance", AuthJSON: string(raw)})
	if err != nil {
		return nil, err
	}
	report = map[string]any{"connection_id": conn.ID}
	version := conn.Version
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		_, e := u.RevokeModelConnection(cleanup, conn.ID, version)
		if e != nil {
			err = errors.Join(err, errors.New("connection cleanup failed"))
		} else {
			report["connection_revoked"] = true
		}
	}()
	rows, err := u.ListModelConnections(ctx)
	if err != nil {
		return report, err
	}
	found := false
	for _, row := range rows {
		if row.ID == conn.ID {
			found = true
		}
	}
	if !found {
		return report, errors.New("connection missing")
	}
	config := sdk.AgentConfiguration{DisplayName: "Go SDK OAuth verification", AgentType: "codex", ModelConnectionID: conn.ID}
	ws, err := u.EnsureAgent(ctx, "go-oauth-verification", config)
	if err != nil {
		return report, err
	}
	report["workspace_id"] = ws.ID
	again, err := u.EnsureAgent(ctx, "go-oauth-verification", config)
	if err != nil || again.ID != ws.ID {
		return report, errors.New("ensure identity mismatch")
	}
	session, err := u.CreateSession(ctx, ws.ID, "Go SDK real inference")
	if err != nil {
		return report, err
	}
	report["session_id"] = session.ID
	turn, err := u.RunTurn(ctx, session.ID, "Reply exactly GO_SDK_OAUTH_OK. Do not use tools.", fmt.Sprint(time.Now().UnixNano()))
	if err != nil {
		return report, err
	}
	report["turn_id"] = turn.TurnID
	first, err := consume(ctx, u, session.ID, 0, true)
	if err != nil {
		return report, err
	}
	rest, err := consume(ctx, u, session.ID, first[len(first)-1].ID, false)
	if err != nil {
		return report, err
	}
	all := append(first, rest...)
	report["events"] = all
	if all[len(all)-1].Type != "turn.completed" {
		return report, errors.New("real turn did not complete")
	}
	messages, err := u.ListMessages(ctx, session.ID)
	if err != nil {
		return report, err
	}
	found = false
	for _, m := range messages {
		if m.Role == "assistant" && strings.Contains(m.ContentJSON, "GO_SDK_OAUTH_OK") {
			found = true
		}
	}
	if !found {
		return report, errors.New("real model reply absent")
	}
	report["model_inference_verified"] = true
	report["history_verified"] = true
	report["reconnect_verified"] = true
	_, err = u.RunTurn(ctx, session.ID, "Write at least 5000 words on mathematics. Do not use tools.", fmt.Sprint(time.Now().UnixNano()))
	if err != nil {
		return report, err
	}
	_, err = u.Interrupt(ctx, session.ID)
	if err != nil {
		return report, err
	}
	interrupted, err := consume(ctx, u, session.ID, all[len(all)-1].ID, false)
	if err != nil {
		return report, err
	}
	report["interrupt_events"] = interrupted
	if interrupted[len(interrupted)-1].Type != "turn.interrupted" {
		return report, errors.New("interrupt not observed")
	}
	report["interrupt_verified"] = true
	updated, err := u.UpdateModelConnection(ctx, conn.ID, sdk.ModelConnectionUpdate{Name: "Go SDK verified", ExpectedVersion: version})
	if err != nil {
		return report, err
	}
	version = updated.Version
	return report, nil
}
func main() {
	var in input
	if json.NewDecoder(os.Stdin).Decode(&in) != nil {
		fmt.Fprintln(os.Stderr, "invalid stdin JSON")
		os.Exit(1)
	}
	report, err := run(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = json.NewEncoder(os.Stderr).Encode(report)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(report)
}
