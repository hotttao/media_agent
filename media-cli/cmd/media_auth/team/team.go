// team.go — media_auth team 接口 e2e 子命令（typed client + 持久化 cookie middleware）。
//
// 覆盖 team CRUD + 成员管理 + 邀请签发/接受的关键路径，验证 HTTP 层 + cookie 鉴权。
// 需先 `media_auth auth login` 拿 cookie（cookie store 跨子命令共享）。
package team

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	teampb "media_agent/hertz_gen/model/media_auth/team"
	mediamauth "media_agent/media_cli/cmd/media_auth"
)

// TeamCmd 是 media_auth 下的 team 二级子命令。
var TeamCmd = &cobra.Command{
	Use:   "team",
	Short: "media_auth TeamService 接口 e2e（create/list/get/members/invite/accept）",
}

func init() {
	TeamCmd.AddCommand(
		createCmd(), listCmd(), getCmd(), deleteCmd(),
		listMembersCmd(), addMemberCmd(), removeMemberCmd(),
		createInviteCmd(), acceptInviteCmd(),
	)
	mediamauth.MediaAuthCmd.AddCommand(TeamCmd)
}

func createCmd() *cobra.Command {
	var name, description string
	c := &cobra.Command{
		Use:   "create",
		Short: "POST /api/teams — 建团（创建者成为首任 admin）",
		RunE: func(c *cobra.Command, args []string) error {
			req := &teampb.CreateTeamRequest{Name: name}
			if description != "" {
				req.Description = &description
			}
			resp, _, err := Client().CreateTeam(context.Background(), req)
			if err != nil {
				return fmt.Errorf("create team: %w", err)
			}
			printJSON("✅ team created", resp)
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "团队名")
	c.Flags().StringVar(&description, "description", "", "描述（可选）")
	_ = c.MarkFlagRequired("name")
	return c
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "GET /api/teams — 列出我所在的 team",
		RunE: func(c *cobra.Command, args []string) error {
			resp, _, err := Client().ListMyTeams(context.Background(), &teampb.ListMyTeamsRequest{})
			if err != nil {
				return fmt.Errorf("list teams: %w", err)
			}
			printJSON("✅ my teams", resp.GetTeams())
			return nil
		},
	}
}

func getCmd() *cobra.Command {
	var id string
	c := &cobra.Command{
		Use:   "get",
		Short: "GET /api/teams/:id — 查 team 详情（仅成员可读）",
		RunE: func(c *cobra.Command, args []string) error {
			resp, _, err := Client().GetTeam(context.Background(), &teampb.GetTeamRequest{Id: id})
			if err != nil {
				return fmt.Errorf("get team: %w", err)
			}
			printJSON("✅ team", resp)
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "team id")
	_ = c.MarkFlagRequired("id")
	return c
}

func deleteCmd() *cobra.Command {
	var id string
	c := &cobra.Command{
		Use:   "delete",
		Short: "DELETE /api/teams/:id — 删 team（admin only）",
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, err := Client().DeleteTeam(context.Background(), &teampb.DeleteTeamRequest{Id: id}); err != nil {
				return fmt.Errorf("delete team: %w", err)
			}
			fmt.Println("✅ team deleted")
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "team id")
	_ = c.MarkFlagRequired("id")
	return c
}

func listMembersCmd() *cobra.Command {
	var id string
	c := &cobra.Command{
		Use:   "members",
		Short: "GET /api/teams/:id/members — 列 team 成员（仅成员可读）",
		RunE: func(c *cobra.Command, args []string) error {
			resp, _, err := Client().ListMembers(context.Background(), &teampb.ListMembersRequest{Id: id})
			if err != nil {
				return fmt.Errorf("list members: %w", err)
			}
			printJSON("✅ members", resp.GetMembers())
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "team id")
	_ = c.MarkFlagRequired("id")
	return c
}

func addMemberCmd() *cobra.Command {
	var id, userID, role string
	c := &cobra.Command{
		Use:   "add-member",
		Short: "POST /api/teams/:id/members — 加成员（admin only）",
		RunE: func(c *cobra.Command, args []string) error {
			req := &teampb.AddMemberRequest{Id: id, UserId: userID}
			if role != "" {
				req.Role = &role
			}
			resp, _, err := Client().AddMember(context.Background(), req)
			if err != nil {
				return fmt.Errorf("add member: %w", err)
			}
			printJSON("✅ member added", resp)
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "team id")
	c.Flags().StringVar(&userID, "user-id", "", "目标 user id")
	c.Flags().StringVar(&role, "role", "", "角色：admin|member（默认 member）")
	_ = c.MarkFlagRequired("id")
	_ = c.MarkFlagRequired("user-id")
	return c
}

func removeMemberCmd() *cobra.Command {
	var id, userID string
	c := &cobra.Command{
		Use:   "remove-member",
		Short: "DELETE /api/teams/:id/members/:userId — 移除成员（admin 或 self）",
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, err := Client().RemoveMember(context.Background(), &teampb.RemoveMemberRequest{Id: id, UserId: userID}); err != nil {
				return fmt.Errorf("remove member: %w", err)
			}
			fmt.Println("✅ member removed")
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "team id")
	c.Flags().StringVar(&userID, "user-id", "", "目标 user id")
	_ = c.MarkFlagRequired("id")
	_ = c.MarkFlagRequired("user-id")
	return c
}

func createInviteCmd() *cobra.Command {
	var id string
	var days int32
	c := &cobra.Command{
		Use:   "create-invite",
		Short: "POST /api/teams/:id/invites — 签发邀请（admin only，默认 7 天）",
		RunE: func(c *cobra.Command, args []string) error {
			req := &teampb.CreateInviteRequest{Id: id}
			if days > 0 {
				req.ExpiresInDays = &days
			}
			resp, _, err := Client().CreateInvite(context.Background(), req)
			if err != nil {
				return fmt.Errorf("create invite: %w", err)
			}
			printJSON("✅ invite created", resp)
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "team id")
	c.Flags().Int32Var(&days, "days", 0, "有效期天数（默认 7）")
	_ = c.MarkFlagRequired("id")
	return c
}

func acceptInviteCmd() *cobra.Command {
	var token string
	c := &cobra.Command{
		Use:   "accept-invite",
		Short: "POST /api/invites/:token/accept — 接受邀请入团（幂等）",
		RunE: func(c *cobra.Command, args []string) error {
			resp, _, err := Client().AcceptInvite(context.Background(), &teampb.AcceptInviteRequest{Token: token})
			if err != nil {
				return fmt.Errorf("accept invite: %w", err)
			}
			printJSON("✅ invite accepted", resp)
			return nil
		},
	}
	c.Flags().StringVar(&token, "token", "", "邀请 token（tinv_xxx）")
	_ = c.MarkFlagRequired("token")
	return c
}

// printJSON 美化打印响应。
func printJSON(prefix string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(prefix)
		return
	}
	fmt.Printf("%s:\n%s\n", prefix, string(b))
}
