// Package hello 承载 HelloService 的 CLI 子命令（二级 service 父命令）。
//
// 本包是 media_example 的子包：
//   - 通过 init() 把 HelloCmd 挂到父包 media_example 的 MediaExampleCmd；
//   - method 子命令（method1.go 等）通过各自的 init() 把自己挂到 HelloCmd。
//
// 调用形态：media-cli media_example hello <method> [flags]
package hello

import (
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd/media_example"
)

// HelloCmd 是二级子命令 hello（HelloService），承载其下所有 RPC method。
// 导出（首字母大写）供 method1.go 等同包子命令的 init() 挂载。
var HelloCmd = &cobra.Command{
	Use:   "hello",
	Short: "调用 HelloService（id=hello）",
	Long: "HelloService 定义在 idl/media_example/hello.proto。\n" +
		"每个 RPC 一个三级子命令，例如：media-cli media_example hello method1 --name=world",
}

func init() {
	media_example.MediaExampleCmd.AddCommand(HelloCmd)
}
