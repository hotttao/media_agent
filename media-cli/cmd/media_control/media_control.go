// Package media_control exposes user-facing drivers for Media Control APIs.
package media_control

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"media_agent/hertz_gen/media_control/teamwork_service"
	"media_agent/hertz_gen/media_control/workspace_service"
	teamworkpb "media_agent/hertz_gen/model/media_control/teamwork"
	workspacepb "media_agent/hertz_gen/model/media_control/workspace"
	"media_agent/hertz_infra/clienthertz"
	config "media_agent/hertz_infra/config"
	"media_agent/media_cli/cmd"
	"media_agent/media_cli/cmd/cookie"
)

var (
	workspaceClient workspace_service.Client
	teamworkClient  teamwork_service.Client
)

// MediaControlCmd groups Media Control command E2E entry points.
var MediaControlCmd = &cobra.Command{Use: "media_control", Short: "调用 Media Control API"}

func init() {
	MediaControlCmd.AddCommand(workspaceCmd(), teamworkCmd())
	cmd.RegisterCmd(MediaControlCmd)
}

// Init builds generated typed clients with the cross-process cookie store.
func Init(cfg *config.Config) {
	clientCfg := cfg.GetClient()["media_control"]
	if clientCfg == nil || !clientCfg.GetEnabled() {
		log.Fatalf("media_control: client.media_control 未配置或 enabled=false")
	}
	host := clientCfg.GetBaseDomain()
	if host == "" {
		log.Fatalf("media_control: client.media_control.base_domain 为空")
	}
	base := clienthertz.NewClientSuiteFromConfig(cfg)
	suite, err := base.NewAppClientSuite("media_control")
	if err != nil {
		log.Fatalf("media_control: derive downstream suite: %v", err)
	}
	shared, err := suite.NewClient()
	if err != nil {
		log.Fatalf("media_control: build shared client: %v", err)
	}
	store := cookie.New(cmd.CookiesDir)
	workspaceClient, err = workspace_service.NewWorkspaceServiceClient(host,
		workspace_service.WithHertzClient(shared),
		workspace_service.WithHertzClientMiddleware(store.Middleware(host)),
	)
	if err != nil {
		log.Fatalf("media_control: create workspace client: %v", err)
	}

	shared, err = suite.NewClient()
	if err != nil {
		log.Fatalf("media_control: build teamwork client: %v", err)
	}
	teamworkClient, err = teamwork_service.NewTeamworkServiceClient(host,
		teamwork_service.WithHertzClient(shared),
		teamwork_service.WithHertzClientMiddleware(store.Middleware(host)),
	)
	if err != nil {
		log.Fatalf("media_control: create teamwork client: %v", err)
	}
}

func printJSON(command *cobra.Command, value any) error {
	encoder := json.NewEncoder(command.OutOrStdout())
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func workspaceCmd() *cobra.Command {
	root := &cobra.Command{Use: "workspace", Short: "Workspace 配置命令"}
	root.AddCommand(createWorkspaceCmd(), listWorkspacesCmd(), getWorkspaceCmd(), updateWorkspaceCmd(), runtimeWorkspaceContextCmd())
	return root
}

func createWorkspaceCmd() *cobra.Command {
	var name, agentType, apiKey string
	command := &cobra.Command{Use: "create", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := workspaceClient.CreateWorkspace(context.Background(), &workspacepb.CreateWorkspaceRequest{Name: name, AgentType: agentType, ApiKey: apiKey})
		if err != nil {
			return fmt.Errorf("create workspace: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&name, "name", "", "Workspace 名称")
	command.Flags().StringVar(&agentType, "agent-type", "", "Agent 类型")
	command.Flags().StringVar(&apiKey, "api-key", "", "Agent API key")
	_ = command.MarkFlagRequired("name")
	_ = command.MarkFlagRequired("agent-type")
	_ = command.MarkFlagRequired("api-key")
	return command
}

func listWorkspacesCmd() *cobra.Command {
	return &cobra.Command{Use: "list", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := workspaceClient.ListWorkspaces(context.Background(), &workspacepb.ListWorkspacesRequest{})
		if err != nil {
			return fmt.Errorf("list workspaces: %w", err)
		}
		return printJSON(command, response)
	}}
}

func getWorkspaceCmd() *cobra.Command {
	var id string
	command := &cobra.Command{Use: "get", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := workspaceClient.GetWorkspace(context.Background(), &workspacepb.GetWorkspaceRequest{WorkspaceId: id})
		if err != nil {
			return fmt.Errorf("get workspace: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&id, "workspace-id", "", "Workspace ID")
	_ = command.MarkFlagRequired("workspace-id")
	return command
}

func updateWorkspaceCmd() *cobra.Command {
	var id, name, agentType, apiKey string
	command := &cobra.Command{Use: "update", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := workspaceClient.UpdateWorkspace(context.Background(), &workspacepb.UpdateWorkspaceRequest{WorkspaceId: id, Name: name, AgentType: agentType, ApiKey: apiKey})
		if err != nil {
			return fmt.Errorf("update workspace: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&id, "workspace-id", "", "Workspace ID")
	command.Flags().StringVar(&name, "name", "", "Workspace 名称")
	command.Flags().StringVar(&agentType, "agent-type", "", "Agent 类型")
	command.Flags().StringVar(&apiKey, "api-key", "", "Agent API key")
	_ = command.MarkFlagRequired("workspace-id")
	_ = command.MarkFlagRequired("name")
	_ = command.MarkFlagRequired("agent-type")
	_ = command.MarkFlagRequired("api-key")
	return command
}

func runtimeWorkspaceContextCmd() *cobra.Command {
	var id string
	command := &cobra.Command{Use: "runtime-context", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := workspaceClient.GetRuntimeContext(context.Background(), &workspacepb.GetRuntimeContextRequest{WorkspaceId: id})
		if err != nil {
			return fmt.Errorf("get workspace runtime context: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&id, "workspace-id", "", "Workspace ID")
	_ = command.MarkFlagRequired("workspace-id")
	return command
}

func teamworkCmd() *cobra.Command {
	root := &cobra.Command{Use: "teamwork", Short: "Teamwork Topology 命令"}
	root.AddCommand(createTopologyCmd(), listTopologiesCmd(), getTopologyCmd(), updateTopologyCmd(), deleteTopologyCmd(), runtimeTopologyContextCmd())
	return root
}

func createTopologyCmd() *cobra.Command {
	var workspaceID, name, entryMemberID, membersJSON, edgesJSON string
	command := &cobra.Command{Use: "create", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := teamworkClient.CreateTopology(context.Background(), &teamworkpb.CreateTopologyRequest{WorkspaceId: workspaceID, Name: name, EntryMemberId: entryMemberID, MembersJson: membersJSON, EdgesJson: edgesJSON})
		if err != nil {
			return fmt.Errorf("create topology: %w", err)
		}
		return printJSON(command, response)
	}}
	addTopologyFlags(command, &name, &entryMemberID, &membersJSON, &edgesJSON)
	command.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace ID")
	_ = command.MarkFlagRequired("workspace-id")
	return command
}

func listTopologiesCmd() *cobra.Command {
	var workspaceID string
	command := &cobra.Command{Use: "list", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := teamworkClient.ListTopologies(context.Background(), &teamworkpb.ListTopologiesRequest{WorkspaceId: workspaceID})
		if err != nil {
			return fmt.Errorf("list topologies: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace ID")
	_ = command.MarkFlagRequired("workspace-id")
	return command
}

func getTopologyCmd() *cobra.Command {
	var id string
	command := &cobra.Command{Use: "get", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := teamworkClient.GetTopology(context.Background(), &teamworkpb.GetTopologyRequest{TopologyId: id})
		if err != nil {
			return fmt.Errorf("get topology: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&id, "topology-id", "", "Topology ID")
	_ = command.MarkFlagRequired("topology-id")
	return command
}

func updateTopologyCmd() *cobra.Command {
	var id, name, entryMemberID, membersJSON, edgesJSON string
	var expectedVersion int64
	command := &cobra.Command{Use: "update", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := teamworkClient.UpdateTopology(context.Background(), &teamworkpb.UpdateTopologyRequest{TopologyId: id, Name: name, EntryMemberId: entryMemberID, MembersJson: membersJSON, EdgesJson: edgesJSON, ExpectedVersion: expectedVersion})
		if err != nil {
			return fmt.Errorf("update topology: %w", err)
		}
		return printJSON(command, response)
	}}
	addTopologyFlags(command, &name, &entryMemberID, &membersJSON, &edgesJSON)
	command.Flags().StringVar(&id, "topology-id", "", "Topology ID")
	command.Flags().Int64Var(&expectedVersion, "expected-version", 0, "期望版本")
	_ = command.MarkFlagRequired("topology-id")
	_ = command.MarkFlagRequired("expected-version")
	return command
}

func deleteTopologyCmd() *cobra.Command {
	var id string
	var expectedVersion int64
	command := &cobra.Command{Use: "delete", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := teamworkClient.DeleteTopology(context.Background(), &teamworkpb.DeleteTopologyRequest{TopologyId: id, ExpectedVersion: expectedVersion})
		if err != nil {
			return fmt.Errorf("delete topology: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&id, "topology-id", "", "Topology ID")
	command.Flags().Int64Var(&expectedVersion, "expected-version", 0, "期望版本")
	_ = command.MarkFlagRequired("topology-id")
	_ = command.MarkFlagRequired("expected-version")
	return command
}

func runtimeTopologyContextCmd() *cobra.Command {
	var id string
	command := &cobra.Command{Use: "runtime-context", RunE: func(command *cobra.Command, _ []string) error {
		response, _, err := teamworkClient.GetRuntimeContext(context.Background(), &teamworkpb.GetRuntimeContextRequest{TopologyId: id})
		if err != nil {
			return fmt.Errorf("get topology runtime context: %w", err)
		}
		return printJSON(command, response)
	}}
	command.Flags().StringVar(&id, "topology-id", "", "Topology ID")
	_ = command.MarkFlagRequired("topology-id")
	return command
}

func addTopologyFlags(command *cobra.Command, name, entryMemberID, membersJSON, edgesJSON *string) {
	command.Flags().StringVar(name, "name", "", "Topology 名称")
	command.Flags().StringVar(entryMemberID, "entry-member-id", "", "入口成员 ID")
	command.Flags().StringVar(membersJSON, "members-json", "", "成员 JSON")
	command.Flags().StringVar(edgesJSON, "edges-json", "", "边 JSON")
	_ = command.MarkFlagRequired("name")
	_ = command.MarkFlagRequired("entry-member-id")
	_ = command.MarkFlagRequired("members-json")
	_ = command.MarkFlagRequired("edges-json")
}
