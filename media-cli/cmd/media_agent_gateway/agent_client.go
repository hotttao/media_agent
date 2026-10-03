package media_agent_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	pb "media_agent/hertz_gen/model/media_agent_gateway/agent_client"
	authpb "media_agent/hertz_gen/model/media_auth/api_token"
)

func agentClientCmd() *cobra.Command {
	root := &cobra.Command{Use: "agent-client", Short: "Agent API Token 管理（不是 Session Token）"}
	root.AddCommand(createAgentTokenCmd())
	return root
}

func createAgentTokenCmd() *cobra.Command {
	var id, name, expiry, key, path string
	command := &cobra.Command{Use: "create-token", RunE: func(c *cobra.Command, _ []string) error {
		if agentClient == nil || tokenManager == nil {
			return fmt.Errorf("Agent Client is not configured")
		}
		if path == "" {
			return fmt.Errorf("--env-out must be a new private file")
		}
		if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
			return fmt.Errorf("--env-out must be a new private file")
		}
		result, _, err := agentClient.CreateAgentAPIToken(context.Background(), &pb.CreateAgentAPITokenRequest{AgentConfigurationId: id, Name: name, ExpiresAt: expiry, IdempotencyKey: key})
		if err != nil {
			return fmt.Errorf("Agent Token request failed; query metadata before retrying or rotating")
		}
		if result == nil || result.GetApiToken() == nil || result.GetApiToken().GetId() == "" || result.GetSecret() == "" {
			return fmt.Errorf("Agent Token response incomplete; query metadata")
		}
		if err := writeAgentSecret(path, result.GetSecret()); err != nil {
			_, _, revokeErr := tokenManager.RevokeApiToken(context.Background(), &authpb.RevokeApiTokenRequest{Id: result.GetApiToken().GetId()})
			if revokeErr != nil {
				return errors.Join(err, fmt.Errorf("could not confirm revocation of token %s; revoke explicitly", result.GetApiToken().GetId()))
			}
			return err
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(result.GetApiToken())
	}}
	command.Flags().StringVar(&id, "agent-configuration-id", "", "稳定 Agent 配置 ID")
	command.Flags().StringVar(&name, "name", "", "Token 名称")
	command.Flags().StringVar(&expiry, "expires-at", "", "RFC3339 到期时间")
	command.Flags().StringVar(&key, "idempotency-key", "", "本次申请的稳定幂等键")
	command.Flags().StringVar(&path, "env-out", "", "一次性 Secret 新文件（0600）")
	for _, flag := range []string{"agent-configuration-id", "name", "expires-at", "idempotency-key", "env-out"} {
		_ = command.MarkFlagRequired(flag)
	}
	return command
}

func writeAgentSecret(path, secret string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("cannot create private Secret file")
	}
	_, writeErr := fmt.Fprintf(f, "MEDIA_API_TOKEN=%s\n", secret)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("cannot persist private Secret file")
	}
	return nil
}
