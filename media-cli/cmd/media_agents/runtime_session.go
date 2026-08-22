package media_agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	runtimeSessionCmd.AddCommand(registerRuntimeSessionCmd, getRuntimeSessionCmd)
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
