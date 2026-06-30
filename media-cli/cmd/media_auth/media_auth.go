// media_auth.go — media_auth 服务 e2e 测试命令（一级子命令）。
//
// 测试分工（harness/contributing/e2e.md）：
//   - 接口测试（需启动服务、打 HTTP）→ media-cli（本目录）；
//   - 接口无关测试（domain 逻辑/算法/迁移）→ media_auth/e2e（go test）。
//
// auth 接口是 session-based（login 下发 cookie，me/change-password 依赖 cookie），
// generated typed client 不维护 cookie jar，故用裸 net/http + cookie jar 直打 HTTP，
// 验证真实 cookie 行为。被测 server 由人/脚本启动（go run ./media_auth），media-cli 只发请求。
package media_auth

import (
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// MediaAuthCmd 是一级子命令 media_auth，承载 media_auth 各 service 的 e2e 测试子命令。
var MediaAuthCmd = &cobra.Command{
	Use:   "media_auth",
	Short: "media_auth 服务接口 e2e 测试（打真实 media_auth server）",
}

func init() {
	cmd.RegisterCmd(MediaAuthCmd)
}
