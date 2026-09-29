package worker_identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	apiClient "media_agent/hertz_gen/media_auth/api_token_service"
	client "media_agent/hertz_gen/media_auth/worker_identity_service"
	api "media_agent/hertz_gen/model/media_auth/api_token"
	pb "media_agent/hertz_gen/model/media_auth/worker_identity"
	"media_agent/hertz_infra/clienthertz"
	"media_agent/hertz_infra/config"
	"media_agent/media_cli/cmd"
	"media_agent/media_cli/cmd/cookie"
	authcmd "media_agent/media_cli/cmd/media_auth"
)

var cli client.Client
var tokens apiClient.Client

func Init(cfg *config.Config) error {
	suite, err := clienthertz.NewClientSuiteFromConfig(cfg).NewAppClientSuite("media_auth")
	if err != nil {
		return err
	}
	shared, err := suite.NewClient()
	if err != nil {
		return err
	}
	host := cfg.GetClient()["media_auth"].GetBaseDomain()
	cli, err = client.NewWorkerIdentityServiceClient(host, client.WithHertzClient(shared), client.WithHertzClientMiddleware(cookie.New(cmd.CookiesDir).Middleware(host)))
	if err != nil {
		return err
	}
	tokens, err = apiClient.NewApiTokenServiceClient(host, apiClient.WithHertzClient(shared), apiClient.WithHertzClientMiddleware(cookie.New(cmd.CookiesDir).Middleware(host)))
	return err
}

func init() {
	group := &cobra.Command{Use: "worker-identity", Short: "Personal Worker 稳定身份及受限 API Token"}
	group.AddCommand(createCmd(), listCmd(), getCmd(), statusCmd(false), statusCmd(true), createTokenCmd())
	authcmd.MediaAuthCmd.AddCommand(group)
}
func output(c *cobra.Command, value any) error { return json.NewEncoder(c.OutOrStdout()).Encode(value) }
func configured() error {
	if cli == nil {
		return errors.New("client.media_auth is not configured")
	}
	return nil
}
func createCmd() *cobra.Command {
	var name, org string
	c := &cobra.Command{Use: "create", RunE: func(c *cobra.Command, _ []string) error {
		if err := configured(); err != nil {
			return err
		}
		resp, _, err := cli.CreateWorkerIdentity(c.Context(), &pb.CreateWorkerIdentityRequest{Name: name, OrganizationId: org})
		if err != nil {
			return err
		}
		return output(c, resp.GetWorkerIdentity())
	}}
	c.Flags().StringVar(&name, "name", "", "Worker 名称")
	c.Flags().StringVar(&org, "organization-id", "", "所属企业ID（不可变更）")
	_ = c.MarkFlagRequired("name")
	_ = c.MarkFlagRequired("organization-id")
	return c
}
func listCmd() *cobra.Command {
	return &cobra.Command{Use: "list", RunE: func(c *cobra.Command, _ []string) error {
		if err := configured(); err != nil {
			return err
		}
		resp, _, err := cli.ListWorkerIdentities(c.Context(), &pb.ListWorkerIdentitiesRequest{})
		if err != nil {
			return err
		}
		return output(c, resp)
	}}
}
func getCmd() *cobra.Command {
	var id string
	c := &cobra.Command{Use: "get", RunE: func(c *cobra.Command, _ []string) error {
		if err := configured(); err != nil {
			return err
		}
		resp, _, err := cli.GetWorkerIdentity(c.Context(), &pb.WorkerIdentityRequest{Id: id})
		if err != nil {
			return err
		}
		return output(c, resp.GetWorkerIdentity())
	}}
	c.Flags().StringVar(&id, "id", "", "Worker Identity ID")
	_ = c.MarkFlagRequired("id")
	return c
}
func statusCmd(deleted bool) *cobra.Command {
	var id string
	name := "disable"
	if deleted {
		name = "delete"
	}
	c := &cobra.Command{Use: name, RunE: func(c *cobra.Command, _ []string) error {
		if err := configured(); err != nil {
			return err
		}
		req := &pb.WorkerIdentityRequest{Id: id}
		var resp *pb.WorkerIdentityStatusResponse
		var err error
		if deleted {
			resp, _, err = cli.DeleteWorkerIdentity(c.Context(), req)
		} else {
			resp, _, err = cli.DisableWorkerIdentity(c.Context(), req)
		}
		if err != nil {
			return err
		}
		return output(c, resp)
	}}
	c.Flags().StringVar(&id, "id", "", "Worker Identity ID")
	_ = c.MarkFlagRequired("id")
	return c
}
func createTokenCmd() *cobra.Command {
	var id, name, expiry, key, envOut string
	var scopes []string
	c := &cobra.Command{Use: "create-token", RunE: func(c *cobra.Command, _ []string) error {
		if err := configured(); err != nil {
			return err
		}
		if tokens == nil {
			return errors.New("api token client is not configured")
		}
		if _, err := os.Stat(envOut); err == nil || !os.IsNotExist(err) {
			return errors.New("env output must be a new file")
		}
		resp, _, err := cli.CreateWorkerApiToken(c.Context(), &pb.CreateWorkerApiTokenRequest{Id: id, Name: name, ExpiresAt: expiry, Scopes: scopes, IdempotencyKey: key})
		if err != nil {
			return err
		}
		if err := saveSecret(envOut, resp.GetSecret()); err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Context()), 15*time.Second)
			defer cancel()
			_, _, revokeErr := tokens.RevokeApiToken(cleanupCtx, &api.RevokeApiTokenRequest{Id: resp.GetApiToken().GetId()})
			return errors.Join(fmt.Errorf("save worker token %s: %w", resp.GetApiToken().GetId(), err), revokeErr)
		}
		return output(c, resp.GetApiToken())
	}}
	c.Flags().StringVar(&id, "id", "", "Worker Identity ID")
	c.Flags().StringVar(&name, "name", "", "Token 名称")
	c.Flags().StringVar(&expiry, "expires-at", "", "RFC3339 到期时间")
	c.Flags().StringVar(&key, "idempotency-key", "", "签发请求去重键")
	c.Flags().StringSliceVar(&scopes, "scopes", nil, "worker scope 子集（默认全部）")
	c.Flags().StringVar(&envOut, "env-out", "", "一次性Secret新文件（0600）")
	for _, flag := range []string{"id", "name", "expires-at", "env-out"} {
		_ = c.MarkFlagRequired(flag)
	}
	return c
}
func saveSecret(path, secret string) error {
	if secret == "" {
		return errors.New("worker token secret is missing")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(f, "MEDIA_API_TOKEN=%s\n", secret)
	return errors.Join(writeErr, f.Close())
}
