// user.go - media_auth 用户管理 e2e 子命令（admin 鉴权，需先 login admin）。
package user

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	userpb "media_agent/hertz_gen/model/media_auth/user"
	mediamauth "media_agent/media_cli/cmd/media_auth"
)

var UserCmd = &cobra.Command{
	Use:   "user",
	Short: "media_auth UserService 接口 e2e（list/create/delete/change-password，需 admin）",
}

func init() {
	UserCmd.AddCommand(listCmd(), createCmd(), deleteCmd(), changePasswordCmd())
	mediamauth.MediaAuthCmd.AddCommand(UserCmd)
}

func listCmd() *cobra.Command {
	var q, sort, order string
	var page, pageSize int32
	c := &cobra.Command{
		Use:   "list",
		Short: "GET /api/users - 分页+搜索用户（admin）",
		RunE: func(c *cobra.Command, args []string) error {
			resp, _, err := Client().ListUsers(context.Background(), &userpb.ListUsersRequest{
				Q: q, Sort: sort, Order: order, Page: page, PageSize: pageSize,
			})
			if err != nil {
				return fmt.Errorf("list users: %w", err)
			}
			printJSON(fmt.Sprintf("✅ users (total=%d)", resp.GetTotal()), resp.GetItems())
			return nil
		},
	}
	c.Flags().StringVar(&q, "q", "", "模糊搜索")
	c.Flags().StringVar(&sort, "sort", "", "created|name|last_active")
	c.Flags().StringVar(&order, "order", "", "asc|desc")
	c.Flags().Int32Var(&page, "page", 1, "页码")
	c.Flags().Int32Var(&pageSize, "page-size", 10, "每页条数")
	return c
}

func createCmd() *cobra.Command {
	var username, displayName, password, email, role string
	c := &cobra.Command{
		Use:   "create",
		Short: "POST /api/users - 新建用户（admin）",
		RunE: func(c *cobra.Command, args []string) error {
			req := &userpb.CreateUserRequest{Username: username, DisplayName: displayName, Password: password}
			if email != "" {
				req.Email = &email
			}
			if role != "" {
				req.Role = &role
			}
			resp, _, err := Client().CreateUser(context.Background(), req)
			if err != nil {
				return fmt.Errorf("create user: %w", err)
			}
			printJSON("✅ user created", resp)
			return nil
		},
	}
	c.Flags().StringVar(&username, "username", "", "用户名")
	c.Flags().StringVar(&displayName, "display-name", "", "展示名")
	c.Flags().StringVar(&password, "password", "", "密码（≥6）")
	c.Flags().StringVar(&email, "email", "", "邮箱（可选）")
	c.Flags().StringVar(&role, "role", "", "user|admin（默认 user）")
	_ = c.MarkFlagRequired("username")
	_ = c.MarkFlagRequired("password")
	return c
}

func deleteCmd() *cobra.Command {
	var id string
	c := &cobra.Command{
		Use:   "delete",
		Short: "DELETE /api/users/:id - 删用户（admin，不能删自己）",
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, err := Client().DeleteUser(context.Background(), &userpb.DeleteUserRequest{Id: id}); err != nil {
				return fmt.Errorf("delete user: %w", err)
			}
			fmt.Println("✅ user deleted")
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "用户 id")
	_ = c.MarkFlagRequired("id")
	return c
}

func changePasswordCmd() *cobra.Command {
	var id, password string
	c := &cobra.Command{
		Use:   "change-password",
		Short: "PUT /api/users/:id/password - admin 改他人密码",
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, err := Client().ChangeUserPassword(context.Background(), &userpb.ChangeUserPasswordRequest{Id: id, Password: password}); err != nil {
				return fmt.Errorf("change password: %w", err)
			}
			fmt.Println("✅ password changed")
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "用户 id")
	c.Flags().StringVar(&password, "password", "", "新密码（≥6）")
	_ = c.MarkFlagRequired("id")
	_ = c.MarkFlagRequired("password")
	return c
}

func printJSON(prefix string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Printf("%s:\n%s\n", prefix, string(b))
}
