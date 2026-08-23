// Package media_agent_gateway exposes user-facing drivers for Agent Gateway APIs.
package media_agent_gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"media_agent/hertz_gen/media_agent_gateway/agent_task_service"
	"media_agent/hertz_gen/media_agent_gateway/chat_service"
	agenttaskpb "media_agent/hertz_gen/model/media_agent_gateway/agent_task"
	chatpb "media_agent/hertz_gen/model/media_agent_gateway/chat"
	"media_agent/hertz_infra/clienthertz"
	config "media_agent/hertz_infra/config"
	"media_agent/media_cli/cmd"
	"media_agent/media_cli/cmd/cookie"
)

var (
	agentTaskClient agent_task_service.Client
	chatClient      chat_service.Client
)

// MediaAgentGatewayCmd groups Agent Gateway command E2E entry points.
var MediaAgentGatewayCmd = &cobra.Command{Use: "media_agent_gateway", Short: "调用 Media Agent Gateway API"}

func init() {
	MediaAgentGatewayCmd.AddCommand(agentTaskCmd(), chatCmd())
	cmd.RegisterCmd(MediaAgentGatewayCmd)
}

// Init builds the generated typed client with the cross-process cookie store.
func Init(cfg *config.Config) {
	clientCfg := cfg.GetClient()["media_agent_gateway"]
	if clientCfg == nil || !clientCfg.GetEnabled() {
		log.Fatalf("media_agent_gateway: client.media_agent_gateway 未配置或 enabled=false")
	}
	host := clientCfg.GetBaseDomain()
	if host == "" {
		log.Fatalf("media_agent_gateway: client.media_agent_gateway.base_domain 为空")
	}
	base := clienthertz.NewClientSuiteFromConfig(cfg)
	suite, err := base.NewAppClientSuite("media_agent_gateway")
	if err != nil {
		log.Fatalf("media_agent_gateway: derive downstream suite: %v", err)
	}
	shared, err := suite.NewClient()
	if err != nil {
		log.Fatalf("media_agent_gateway: build shared client: %v", err)
	}
	store := cookie.New(cmd.CookiesDir)
	chatClient, err = chat_service.NewChatServiceClient(host,
		chat_service.WithHertzClient(shared),
		chat_service.WithHertzClientMiddleware(store.Middleware(host)),
	)
	if err != nil {
		log.Fatalf("media_agent_gateway: create chat client: %v", err)
	}
	shared, err = suite.NewClient()
	if err != nil {
		log.Fatalf("media_agent_gateway: build agent task client: %v", err)
	}
	agentTaskClient, err = agent_task_service.NewAgentTaskServiceClient(host,
		agent_task_service.WithHertzClient(shared),
		agent_task_service.WithHertzClientMiddleware(store.Middleware(host)),
	)
	if err != nil {
		log.Fatalf("media_agent_gateway: create agent task client: %v", err)
	}
}

func chatCmd() *cobra.Command {
	root := &cobra.Command{Use: "chat", Short: "Agent Session 与 Turn 命令"}
	sessions := &cobra.Command{Use: "session", Short: "Session 命令"}
	sessions.AddCommand(createSessionCmd(), listSessionsCmd())
	turns := &cobra.Command{Use: "turn", Short: "Turn 命令"}
	turns.AddCommand(runTurnCmd())
	root.AddCommand(sessions, turns)
	return root
}

func createSessionCmd() *cobra.Command {
	var workspaceID, title string
	command := &cobra.Command{Use: "create", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := chatClient.CreateSession(context.Background(), &chatpb.CreateSessionRequest{WorkspaceId: workspaceID, Title: title})
		if err != nil {
			return fmt.Errorf("create chat session: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace ID")
	command.Flags().StringVar(&title, "title", "", "Session 标题")
	_ = command.MarkFlagRequired("workspace-id")
	return command
}

func listSessionsCmd() *cobra.Command {
	var workspaceID string
	command := &cobra.Command{Use: "list", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := chatClient.ListSessions(context.Background(), &chatpb.ListSessionsRequest{WorkspaceId: workspaceID})
		if err != nil {
			return fmt.Errorf("list chat sessions: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace ID")
	_ = command.MarkFlagRequired("workspace-id")
	return command
}

func runTurnCmd() *cobra.Command {
	var sessionID, message, idempotencyKey string
	command := &cobra.Command{Use: "run", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := chatClient.RunTurn(context.Background(), &chatpb.RunTurnRequest{SessionId: sessionID, Message: message, IdempotencyKey: idempotencyKey})
		if err != nil {
			return fmt.Errorf("run chat turn: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&sessionID, "session-id", "", "Session ID")
	command.Flags().StringVar(&message, "message", "", "用户消息")
	command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "幂等键")
	_ = command.MarkFlagRequired("session-id")
	_ = command.MarkFlagRequired("message")
	_ = command.MarkFlagRequired("idempotency-key")
	return command
}

func printJSON(command *cobra.Command, value any) error {
	encoder := json.NewEncoder(command.OutOrStdout())
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func agentTaskCmd() *cobra.Command {
	root := &cobra.Command{Use: "agent-task", Short: "Agent Task 命令"}
	root.AddCommand(createTaskCmd(), listTasksCmd(), getTaskCmd(), cancelTaskCmd())
	return root
}

func createTaskCmd() *cobra.Command {
	var workspaceID, sessionID, source, resourceID, key, inputJSON, topologyID string
	var topologyVersion int64
	command := &cobra.Command{Use: "create", RunE: func(command *cobra.Command, _ []string) error {
		request := &agenttaskpb.CreateTaskRequest{WorkspaceId: workspaceID, SessionId: sessionID, Source: source, SourceResourceId: resourceID, IdempotencyKey: key, InputJson: inputJSON, TopologyId: topologyID, TopologyVersion: topologyVersion}
		response, _, err := agentTaskClient.CreateTask(context.Background(), request)
		if err != nil {
			return fmt.Errorf("create agent task: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace ID")
	command.Flags().StringVar(&sessionID, "session-id", "", "可选 Session ID")
	command.Flags().StringVar(&source, "source", "", "任务来源")
	command.Flags().StringVar(&resourceID, "source-resource-id", "", "来源资源 ID")
	command.Flags().StringVar(&key, "idempotency-key", "", "幂等键")
	command.Flags().StringVar(&inputJSON, "input-json", "", "输入 JSON")
	command.Flags().StringVar(&topologyID, "topology-id", "", "可选 Topology ID")
	command.Flags().Int64Var(&topologyVersion, "topology-version", 0, "Topology 版本")
	_ = command.MarkFlagRequired("workspace-id")
	_ = command.MarkFlagRequired("source")
	_ = command.MarkFlagRequired("source-resource-id")
	_ = command.MarkFlagRequired("idempotency-key")
	_ = command.MarkFlagRequired("input-json")
	return command
}

func listTasksCmd() *cobra.Command {
	var workspaceID, status string
	command := &cobra.Command{Use: "list", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := agentTaskClient.ListTasks(context.Background(), &agenttaskpb.ListTasksRequest{WorkspaceId: workspaceID, Status: status})
		if err != nil {
			return fmt.Errorf("list agent tasks: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace ID")
	command.Flags().StringVar(&status, "status", "", "可选状态")
	_ = command.MarkFlagRequired("workspace-id")
	return command
}

func getTaskCmd() *cobra.Command {
	var id string
	command := &cobra.Command{Use: "get", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := agentTaskClient.GetTask(context.Background(), &agenttaskpb.GetTaskRequest{TaskId: id})
		if err != nil {
			return fmt.Errorf("get agent task: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&id, "task-id", "", "Task ID")
	_ = command.MarkFlagRequired("task-id")
	return command
}

func cancelTaskCmd() *cobra.Command {
	var id string
	command := &cobra.Command{Use: "cancel", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := agentTaskClient.CancelTask(context.Background(), &agenttaskpb.CancelTaskRequest{TaskId: id})
		if err != nil {
			return fmt.Errorf("cancel agent task: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&id, "task-id", "", "Task ID")
	_ = command.MarkFlagRequired("task-id")
	return command
}
