// Package cmd 存放 media-cli 各子命令及根命令。
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "media-cli",
	Short: "media_agent 命令行工具",
	Long: "media-cli 调用 media_agent 各微服务暴露的 API。\n" +
		"既供用户手动使用，也作为 e2e 测试工具。\n" +
		"实现基于 hertz_gen 生成的 typed client 代码。",
}

// FlagHost 全局下游服务地址，各子命令可读取。
var FlagHost string

func init() {
	rootCmd.PersistentFlags().StringVar(&FlagHost, "host", "http://127.0.0.1:8888", "下游服务地址")
}

// RegisterCmd 注册子命令到 rootCmd。
func RegisterCmd(c *cobra.Command) {
	rootCmd.AddCommand(c)
}

// Execute 执行根命令。
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
