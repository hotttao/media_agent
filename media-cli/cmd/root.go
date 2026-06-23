// Package cmd 存放 media-cli 各子命令及根命令。
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// FlagHost 全局下游服务地址，各子命令可读取。
var FlagHost string

// FlagVerbose 详细输出。
var FlagVerbose bool

var rootCmd = &cobra.Command{
	Use:   "media-cli",
	Short: "media_agent 命令行工具",
	Long: "media-cli 调用 media_agent 各微服务暴露的 API。\n" +
		"既供用户手动使用，也作为 hertz_infra 的 e2e 测试工具。\n" +
		"实现基于 hertz_infra 治理层 + hertz_gen 生成的 typed client 代码。",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&FlagHost, "host", "http://127.0.0.1:8888", "被测服务地址")
	rootCmd.PersistentFlags().BoolVarP(&FlagVerbose, "verbose", "v", false, "详细输出")
}

// RegisterCmd 注册子命令到 rootCmd。
func RegisterCmd(c *cobra.Command) {
	rootCmd.AddCommand(c)
}

// cliObsShutdown 由 root PersistentPreRunE 设置，Execute 退出时调用以 flush trace / 日志。
// serve 子命令会覆盖 root 的 PersistentPreRunE（自身经 ServerSuite.InitObservability 初始化），
// 此时本变量为 nil，不重复 shutdown。
var cliObsShutdown func(context.Context) error

func init() {
	// root PersistentPreRunE：为所有「非 serve」子命令初始化 media-cli 可观测性。
	// serve 自带 PersistentPreRunE 覆盖本钩子——它经 ServerSuite 用 loader 配置初始化，
	// 避免与 CLI 可观测性争抢全局 OTel provider（observability.InitOTel 内部 sync.Once）。
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cliObsShutdown = initCLIObservability()
		return nil
	}
}

// Execute 执行根命令。
//
// 非 serve 子命令：root PersistentPreRunE 初始化 media-cli 可观测性，Execute 退出时 shutdown。
// serve 子命令：自带 PersistentPreRunE 跳过 CLI 可观测性，由 ServerSuite.InitObservability 用
// loader 配置初始化（app.name=被测服务名，而非 media-cli）。
func Execute() {
	err := rootCmd.Execute()
	if cliObsShutdown != nil {
		cliObsShutdown(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
