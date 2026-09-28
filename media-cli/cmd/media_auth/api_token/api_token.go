package api_token

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	client "media_agent/hertz_gen/media_auth/api_token_service"
	pb "media_agent/hertz_gen/model/media_auth/api_token"
	"media_agent/hertz_infra/clienthertz"
	"media_agent/hertz_infra/config"
	"media_agent/media_cli/cmd"
	"media_agent/media_cli/cmd/cookie"
	authcmd "media_agent/media_cli/cmd/media_auth"
)

var cli client.Client

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
	cli, err = client.NewApiTokenServiceClient(host, client.WithHertzClient(shared),
		client.WithHertzClientMiddleware(cookie.New(cmd.CookiesDir).Middleware(host)))
	return err
}

func init() {
	group := &cobra.Command{Use: "api-token", Short: "API Token 生命周期（支持 MEDIA_API_TOKEN）"}
	group.AddCommand(createCmd(), listCmd(), rotateCmd(), revokeCmd())
	authcmd.MediaAuthCmd.AddCommand(group)
}

func output(c *cobra.Command, value any) error { return json.NewEncoder(c.OutOrStdout()).Encode(value) }

// Secrets are written only to an explicitly requested private env file.
func saveSecret(path, secret string) error {
	if path == "" {
		return fmt.Errorf("--env-out is required to receive the one-time secret")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "MEDIA_API_TOKEN=%s\n", secret)
	return err
}

func createCmd() *cobra.Command {
	var name, org, expiry, envOut string
	var scopes []string
	c := &cobra.Command{Use: "create", RunE: func(c *cobra.Command, _ []string) error {
		if cli == nil {
			return fmt.Errorf("client.media_auth is not configured")
		}
		if _, err := os.Stat(envOut); err == nil || !os.IsNotExist(err) {
			return fmt.Errorf("env output must be a new file")
		}
		resp, _, err := cli.CreateApiToken(context.Background(), &pb.CreateApiTokenRequest{Name: name, OrganizationId: org, Scopes: scopes, ExpiresAt: expiry})
		if err != nil {
			return err
		}
		if err := saveSecret(envOut, resp.GetSecret()); err != nil {
			_, _, revokeErr := cli.RevokeApiToken(context.Background(), &pb.RevokeApiTokenRequest{Id: resp.GetApiToken().GetId()})
			return errors.Join(fmt.Errorf("save secret for token %s: %w", resp.GetApiToken().GetId(), err), revokeErr)
		}
		return output(c, resp.GetApiToken())
	}}
	c.Flags().StringVar(&name, "name", "", "Token 名称")
	c.Flags().StringVar(&org, "organization-id", "", "组织 ID")
	c.Flags().StringVar(&expiry, "expires-at", "", "RFC3339 到期时间")
	c.Flags().StringSliceVar(&scopes, "scopes", []string{"api.access"}, "允许的 scope")
	c.Flags().StringVar(&envOut, "env-out", "", "一次性 Secret 保存路径（新文件，0600）")
	for _, f := range []string{"name", "expires-at", "env-out"} {
		_ = c.MarkFlagRequired(f)
	}
	return c
}

func listCmd() *cobra.Command {
	return &cobra.Command{Use: "list", RunE: func(c *cobra.Command, _ []string) error {
		if cli == nil {
			return fmt.Errorf("client.media_auth is not configured")
		}
		resp, _, err := cli.ListApiTokens(context.Background(), &pb.ListApiTokensRequest{})
		if err != nil {
			return err
		}
		return output(c, resp)
	}}
}

func rotateCmd() *cobra.Command {
	var id, envOut string
	c := &cobra.Command{Use: "rotate", RunE: func(c *cobra.Command, _ []string) error {
		if cli == nil {
			return fmt.Errorf("client.media_auth is not configured")
		}
		if _, err := os.Stat(envOut); err == nil || !os.IsNotExist(err) {
			return fmt.Errorf("env output must be a new file")
		}
		resp, _, err := cli.RotateApiToken(context.Background(), &pb.RotateApiTokenRequest{Id: id})
		if err != nil {
			return err
		}
		if err := saveSecret(envOut, resp.GetSecret()); err != nil {
			_, _, revokeErr := cli.RevokeApiToken(context.Background(), &pb.RevokeApiTokenRequest{Id: id})
			return errors.Join(err, revokeErr)
		}
		return output(c, resp.GetApiToken())
	}}
	c.Flags().StringVar(&id, "id", "", "API Token ID")
	c.Flags().StringVar(&envOut, "env-out", "", "新 Secret 保存路径（新文件，0600）")
	_ = c.MarkFlagRequired("id")
	_ = c.MarkFlagRequired("env-out")
	return c
}

func revokeCmd() *cobra.Command {
	var id string
	c := &cobra.Command{Use: "revoke", RunE: func(c *cobra.Command, _ []string) error {
		if cli == nil {
			return fmt.Errorf("client.media_auth is not configured")
		}
		resp, _, err := cli.RevokeApiToken(context.Background(), &pb.RevokeApiTokenRequest{Id: id})
		if err != nil {
			return err
		}
		return output(c, resp)
	}}
	c.Flags().StringVar(&id, "id", "", "API Token ID")
	_ = c.MarkFlagRequired("id")
	return c
}
