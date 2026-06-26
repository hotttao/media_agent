// Package media_example 承载 media_example 微服务的 CLI 子命令（一级 app 父命令）。
//
// 目录组织（与子命令树对齐）：
//
//	cmd/media_example/             ← 本包（一级 app 父命令）
//	cmd/media_example/hello/       ← hello service 子包（二级 service）
//	    hello.go                  ← service 父命令 HelloCmd
//	    method1.go                ← RPC 三级子命令
//
// hello 子包通过 init() 把 HelloCmd 挂到 MediaExampleCmd（一级父命令）；
// MediaExampleCmd 必须导出（subpackage 的 init 看不到 unexported 符号）。
package media_example

import (
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// MediaExampleCmd 是一级子命令 media_example，承载本 app 下所有 service。
// 导出（首字母大写）供 media_example/hello subpackage 的 init() 挂载子命令。
var MediaExampleCmd = &cobra.Command{
	Use:   "media_example",
	Short: "调用 media_example 微服务的 service / method",
	Long: "media_example 是 Hertz 模板微服务。\n" +
		"本命令树暴露它的 service 与 method 三层结构：\n" +
		"  media-cli media_example <service> <method> [flags]",
}

// init 把自身挂到 rootCmd；hello 等子包通过各自的 init() 把自己挂到本命令。
func init() {
	cmd.RegisterCmd(MediaExampleCmd)
}
