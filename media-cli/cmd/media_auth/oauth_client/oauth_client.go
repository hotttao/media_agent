// oauth_client.go — media_auth OAuth client 管理 e2e 子命令（typed client + 持久化 cookie）。
//
// 覆盖 create / list / revoke。需先 `media_auth auth login`（admin 账号）。
package oauth_client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	oauthclientpb "media_agent/hertz_gen/model/media_auth/oauth_client"
	mediamauth "media_agent/media_cli/cmd/media_auth"
)

// OAuthClientCmd 是 media_auth 下的 oauth-client 二级子命令。
var OAuthClientCmd = &cobra.Command{
	Use:   "oauth-client",
	Short: "media_auth OAuthClientService 接口 e2e（create/list/revoke，需 admin）",
}

func init() {
	OAuthClientCmd.AddCommand(createCmd(), listCmd(), revokeCmd())
	mediamauth.MediaAuthCmd.AddCommand(OAuthClientCmd)
}

func createCmd() *cobra.Command {
	var name string
	var redirectURIs []string
	var homepageURL, description string
	c := &cobra.Command{
		Use:   "create",
		Short: "POST /api/oauth/clients — 新建 OAuth client（admin，明文 secret 仅本次返回）",
		RunE: func(c *cobra.Command, args []string) error {
			req := &oauthclientpb.CreateClientRequest{Name: name, RedirectUris: redirectURIs}
			if homepageURL != "" {
				req.HomepageUrl = &homepageURL
			}
			if description != "" {
				req.Description = &description
			}
			resp, _, err := Client().CreateClient(context.Background(), req)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			printJSON("✅ client created", resp)
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "client 名")
	c.Flags().StringArrayVar(&redirectURIs, "redirect-uri", nil, "redirect_uri（可多次）")
	c.Flags().StringVar(&homepageURL, "homepage", "", "homepage URL（可选）")
	c.Flags().StringVar(&description, "description", "", "描述（可选）")
	_ = c.MarkFlagRequired("name")
	_ = c.MarkFlagRequired("redirect-uri")
	return c
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "GET /api/oauth/clients — 列全部 OAuth client（admin）",
		RunE: func(c *cobra.Command, args []string) error {
			resp, _, err := Client().ListClients(context.Background(), &oauthclientpb.ListClientsRequest{})
			if err != nil {
				return fmt.Errorf("list clients: %w", err)
			}
			printJSON("✅ clients", resp.GetClients())
			return nil
		},
	}
}

func revokeCmd() *cobra.Command {
	var id string
	c := &cobra.Command{
		Use:   "revoke",
		Short: "DELETE /api/oauth/clients/:id — 删 client（admin，级联清 codes+refresh）",
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, err := Client().RevokeClient(context.Background(), &oauthclientpb.RevokeClientRequest{Id: id}); err != nil {
				return fmt.Errorf("revoke client: %w", err)
			}
			fmt.Println("✅ client revoked")
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "client id")
	_ = c.MarkFlagRequired("id")
	return c
}

func printJSON(prefix string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(prefix)
		return
	}
	fmt.Printf("%s:\n%s\n", prefix, string(b))
}
