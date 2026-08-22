package media_agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

var runtimeSessionCmd = &cobra.Command{Use: "runtime-session", Short: "注册和查询 Agent Runtime Session"}

var (
	registerSessionID      string
	registerContextVersion int64
	registerContextRef     string
	submitTurnSessionID    string
	submitTurnKey          string
	submitTurnContext      int64
	submitTurnInput        string
	getTurnSessionID       string
	getTurnID              string
	interruptTurnSessionID string
	interruptTurnID        string
)

var registerRuntimeSessionCmd = &cobra.Command{
	Use: "register", Short: "幂等注册 Runtime Session",
	RunE: func(command *cobra.Command, _ []string) error {
		payload, err := json.Marshal(map[string]any{
			"session_id": registerSessionID, "context_version": registerContextVersion,
			"context_ref": registerContextRef,
		})
		if err != nil {
			return err
		}
		return requestAndPrint(command, http.MethodPost, "/api/v1/runtime-sessions", payload)
	},
}

var getRuntimeSessionCmd = &cobra.Command{
	Use: "get <session-id>", Short: "查询 Runtime Session", Args: cobra.ExactArgs(1),
	RunE: func(command *cobra.Command, args []string) error {
		return requestAndPrint(command, http.MethodGet, "/api/v1/runtime-sessions/"+args[0], nil)
	},
}

var submitTurnCmd = &cobra.Command{
	Use: "submit-turn", Short: "幂等提交 Turn 到持久化 FIFO",
	RunE: func(command *cobra.Command, _ []string) error {
		var input map[string]any
		if err := json.Unmarshal([]byte(submitTurnInput), &input); err != nil {
			return fmt.Errorf("invalid --input JSON: %w", err)
		}
		payload, err := json.Marshal(map[string]any{
			"idempotency_key": submitTurnKey, "context_version": submitTurnContext, "input": input,
			"api_key": os.Getenv("MEDIA_AGENT_API_KEY"), "session_token": os.Getenv("MEDIA_AGENT_SESSION_TOKEN"),
		})
		if err != nil {
			return err
		}
		return requestAndPrint(command, http.MethodPost,
			"/api/v1/runtime-sessions/"+submitTurnSessionID+"/turns", payload)
	},
}

var getTurnCmd = &cobra.Command{
	Use: "get-turn", Short: "查询持久化 Turn",
	RunE: func(command *cobra.Command, _ []string) error {
		return requestAndPrint(command, http.MethodGet,
			"/api/v1/runtime-sessions/"+getTurnSessionID+"/turns/"+getTurnID, nil)
	},
}

var interruptTurnCmd = &cobra.Command{
	Use: "interrupt-turn", Short: "软中断当前 running Turn",
	RunE: func(command *cobra.Command, _ []string) error {
		return requestAndPrint(command, http.MethodPost,
			"/api/v1/runtime-sessions/"+interruptTurnSessionID+"/turns/"+interruptTurnID+"/interrupt", []byte(`{}`))
	},
}

var runtimeWorkspaceCmd = &cobra.Command{
	Use:   "runtime-workspace",
	Short: "查询 Agent 挂载的 Workspace Runtime Directory identity",
}

var getRuntimeWorkspaceCmd = &cobra.Command{
	Use:   "info",
	Short: "查询当前 Workspace Runtime Directory",
	RunE: func(command *cobra.Command, _ []string) error {
		return requestAndPrint(command, http.MethodGet, "/api/v1/runtime-workspace", nil)
	},
}

func init() {
	registerRuntimeSessionCmd.Flags().StringVar(&registerSessionID, "session-id", "", "Gateway 分配的 Session ID")
	registerRuntimeSessionCmd.Flags().Int64Var(&registerContextVersion, "context-version", 0, "运行上下文版本")
	registerRuntimeSessionCmd.Flags().StringVar(&registerContextRef, "context-ref", "", "不可变运行上下文引用")
	_ = registerRuntimeSessionCmd.MarkFlagRequired("session-id")
	_ = registerRuntimeSessionCmd.MarkFlagRequired("context-version")
	_ = registerRuntimeSessionCmd.MarkFlagRequired("context-ref")
	submitTurnCmd.Flags().StringVar(&submitTurnSessionID, "session-id", "", "Runtime Session ID")
	submitTurnCmd.Flags().StringVar(&submitTurnKey, "idempotency-key", "", "稳定幂等键")
	submitTurnCmd.Flags().Int64Var(&submitTurnContext, "context-version", 0, "运行上下文版本")
	submitTurnCmd.Flags().StringVar(&submitTurnInput, "input", "", "Turn 输入 JSON object")
	_ = submitTurnCmd.MarkFlagRequired("session-id")
	_ = submitTurnCmd.MarkFlagRequired("idempotency-key")
	_ = submitTurnCmd.MarkFlagRequired("context-version")
	_ = submitTurnCmd.MarkFlagRequired("input")
	getTurnCmd.Flags().StringVar(&getTurnSessionID, "session-id", "", "Runtime Session ID")
	getTurnCmd.Flags().StringVar(&getTurnID, "turn-id", "", "Turn ID")
	interruptTurnCmd.Flags().StringVar(&interruptTurnSessionID, "session-id", "", "Runtime Session ID")
	interruptTurnCmd.Flags().StringVar(&interruptTurnID, "turn-id", "", "Turn ID")
	_ = getTurnCmd.MarkFlagRequired("session-id")
	_ = getTurnCmd.MarkFlagRequired("turn-id")
	_ = interruptTurnCmd.MarkFlagRequired("session-id")
	_ = interruptTurnCmd.MarkFlagRequired("turn-id")
	runtimeSessionCmd.AddCommand(registerRuntimeSessionCmd, getRuntimeSessionCmd, submitTurnCmd, getTurnCmd, interruptTurnCmd)
	runtimeWorkspaceCmd.AddCommand(getRuntimeWorkspaceCmd)
	MediaAgentsCmd.AddCommand(runtimeSessionCmd, runtimeWorkspaceCmd)
}

func requestAndPrint(command *cobra.Command, method, path string, payload []byte) error {
	request, err := http.NewRequest(method, strings.TrimRight(cmd.FlagHost, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("agent runtime returned %s: %s", response.Status, body)
	}
	_, err = fmt.Fprintln(command.OutOrStdout(), string(body))
	return err
}
