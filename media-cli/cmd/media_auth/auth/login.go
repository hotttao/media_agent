// login.go — media_auth auth 接口 e2e 子命令（用 typed client + 持久化 cookie middleware）。
//
// cookie 由 client.go 的 middleware 自动注入/收集/落盘，命令只调 typed client 方法。
// login 一次后 cookie 落盘，后续 me/change-password 等命令直接带 cookie，无需重登。
package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	authmodel "media_agent/hertz_gen/model/media_auth/auth"
	"media_agent/media_cli/cmd"
	mediamauth "media_agent/media_cli/cmd/media_auth"
)

// AuthCmd 是 media_auth 下的 auth 二级子命令。
var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "media_auth AuthService 接口 e2e（login/me/logout/change-password/update-me）",
}

func init() {
	AuthCmd.AddCommand(loginCmd(), registerCmd(), meCmd(), logoutCmd(), changePasswordCmd(), updateMeCmd())
	mediamauth.MediaAuthCmd.AddCommand(AuthCmd) // 挂到一级子命令 media_auth
}

// registerCmd POST /api/auth/register - 自服务注册（自动登录下发 cookie）。
func registerCmd() *cobra.Command {
	var username, password, displayName, email string
	c := &cobra.Command{
		Use:   "register",
		Short: "POST /api/auth/register - 自服务注册（自动登录）",
		RunE: func(c *cobra.Command, args []string) error {
			req := &authmodel.RegisterRequest{Username: username, Password: password}
			if displayName != "" {
				req.DisplayName = &displayName
			}
			if email != "" {
				req.Email = &email
			}
			resp, _, err := Client().Register(context.Background(), req)
			if err != nil {
				return fmt.Errorf("register: %w", err)
			}
			fmt.Println("✅ register success (auto-login):")
			printUser(resp.GetUser())
			return nil
		},
	}
	c.Flags().StringVar(&username, "username", "", "用户名")
	c.Flags().StringVar(&password, "password", "", "密码（≥6）")
	c.Flags().StringVar(&displayName, "display-name", "", "展示名（可选，缺省=username）")
	c.Flags().StringVar(&email, "email", "", "邮箱（可选）")
	_ = c.MarkFlagRequired("username")
	_ = c.MarkFlagRequired("password")
	return c
}

// loginCmd POST /api/auth/login {username,password} → 下发 cookie（middleware 落盘）+ 返回 ApiUser。
func loginCmd() *cobra.Command {
	var username, password string
	c := &cobra.Command{
		Use:   "login",
		Short: "POST /api/auth/login — 本地/LDAP 双路登录，下发 session cookie",
		RunE: func(c *cobra.Command, args []string) error {
			resp, _, err := Client().Login(context.Background(), &authmodel.LoginRequest{
				Username: username, Password: password,
			})
			if err != nil {
				return fmt.Errorf("login: %w", err)
			}
			fmt.Println("✅ login success:")
			printUser(resp.GetUser())
			return nil
		},
	}
	c.Flags().StringVar(&username, "username", "", "用户名")
	c.Flags().StringVar(&password, "password", "", "密码")
	_ = c.MarkFlagRequired("username")
	_ = c.MarkFlagRequired("password")
	return c
}

// meCmd GET /api/auth/me → 当前用户档案（依赖 login 落盘的 cookie，middleware 自动带）。
func meCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "GET /api/auth/me — 当前用户档案（需先 login）",
		RunE: func(c *cobra.Command, args []string) error {
			resp, _, err := Client().GetMe(context.Background(), &authmodel.GetMeRequest{})
			if err != nil {
				return fmt.Errorf("me: %w", err)
			}
			fmt.Println("✅ me:")
			printUser(resp)
			return nil
		},
	}
}

// logoutCmd POST /api/auth/logout → 清 cookie（handler 清 + 本地 store 清）。
func logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "POST /api/auth/logout — 清除 session cookie",
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, err := Client().Logout(context.Background(), &authmodel.LogoutRequest{}); err != nil {
				return fmt.Errorf("logout: %w", err)
			}
			// 本地 cookie store 也清（避免后续命令带过期 cookie）。
			if s := CookieStore(); s != nil {
				s.Clear(cmd.FlagHost)
			}
			fmt.Println("✅ logout success")
			return nil
		},
	}
}

// changePasswordCmd PUT /api/auth/me/password {current_password,new_password}（需先 login）。
func changePasswordCmd() *cobra.Command {
	var current, new string
	c := &cobra.Command{
		Use:   "change-password",
		Short: "PUT /api/auth/me/password — 改密（需先 login）",
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, err := Client().ChangePassword(context.Background(), &authmodel.ChangePasswordRequest{
				CurrentPassword: current, NewPassword: new,
			}); err != nil {
				return fmt.Errorf("change-password: %w", err)
			}
			fmt.Println("✅ change-password success")
			return nil
		},
	}
	c.Flags().StringVar(&current, "current", "", "当前密码")
	c.Flags().StringVar(&new, "new", "", "新密码（≥6 位）")
	_ = c.MarkFlagRequired("current")
	_ = c.MarkFlagRequired("new")
	return c
}

// updateMeCmd PATCH /api/auth/me {auto_evolution}（需先 login）。
func updateMeCmd() *cobra.Command {
	var autoEvolution bool
	c := &cobra.Command{
		Use:   "update-me",
		Short: "PATCH /api/auth/me — 更新本人设置（auto_evolution，需先 login）",
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, err := Client().UpdateMe(context.Background(), &authmodel.UpdateMeRequest{
				AutoEvolution: autoEvolution,
			}); err != nil {
				return fmt.Errorf("update-me: %w", err)
			}
			fmt.Println("✅ update-me success")
			return nil
		},
	}
	c.Flags().BoolVar(&autoEvolution, "auto-evolution", false, "auto_evolution 设置值")
	return c
}

// printUser 打印 ApiUser。
func printUser(u *authmodel.ApiUser) {
	if u == nil {
		fmt.Println("  <nil>")
		return
	}
	fmt.Printf("  id=%s username=%s role=%s auth_source=%s default_prompt_id=%s auto_evolution=%v\n",
		u.GetId(), u.GetUsername(), u.GetRole(), u.GetAuthSource(), u.GetDefaultPromptId(), u.GetAutoEvolution())
}
