// Package infra 承载服务端治理能力 e2e 测试的 CLI 子命令。
//
// 二层结构：
//
//	media-cli infra <feature>
//
// 一级 = infra，二级 = 治理功能（health / recovery / metrics / cors / ratelimit /
// requestid / timeout / pprof / accesslog / consul / trace / log）。
//
// 共享 HTTP 工具（DoRequest / Pass / Fail / Skip / HTTPClient）已抽到
// cmd/internal/testharness，本包仅保留 infra 自己的父命令与旗标。
package infra

import (
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// infraCmd 是一级子命令 infra，承载本分类下所有治理功能。
var infraCmd = &cobra.Command{
	Use:   "infra",
	Short: "服务端治理能力 e2e 测试",
}

// init 把自身挂到 rootCmd；治理功能子命令通过各自的 init() 把自己挂到本命令。
func init() {
	cmd.RegisterCmd(infraCmd)
}
