// Package cmd 承载 media-cli 根命令、跨子命令共享的注册入口。
//
// 目录组织约定（与用户确认过的结构对齐）：
//
//	cmd/media_example/<app>/hello/   ← 一级 app / 二级 service（含共享 client init）
//	cmd/infra/<feature>.go            ← infra 主题（含 WithSpan / config-reload 等治理工具与测试）
//	cmd/client/<feature>.go           ← client 主题（e2e 测试）
//
// subpackage 通过 main.go 的 blank import 触发 init()，把各自子命令挂到 rootCmd。

package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "media-cli",
	Short: "media_agent 命令行工具",
	Long: "media-cli 调用 media_agent 各微服务暴露的 API。\n" +
		"既供用户手动使用，也作为 hertz_infra 的 e2e 测试工具。\n" +
		"实现基于 hertz_infra 治理层 + hertz_gen 生成的 typed client 代码。",
}

func init() {
	// 把 rootCmd 的 persistent flag 值绑定到本包的全局 var，
	// 让 subpackage（infra / client / media_example）直接读 FlagHost / FlagVerbose。
	rootCmd.PersistentFlags().StringVar(&FlagHost, "host", "http://127.0.0.1:8888", "被测服务地址")
	rootCmd.PersistentFlags().BoolVarP(&FlagVerbose, "verbose", "v", false, "详细输出")
}

// RegisterCmd 注册子命令到 rootCmd。
//
// 各 subpackage（media_example / infra / client）的 init() 通过
// main.go 的 blank import 触发注册。
func RegisterCmd(c *cobra.Command) {
	rootCmd.AddCommand(c)
}

// Execute 执行根命令。
//
// 设计：observability 初始化（globalhertz.InitLogger + InitOTel）由 main.go 在
// cmd.Execute() 之前完成；shutdown 在 main.go 的 defer 里调用。本包不持有
// shutdown 闭包（避免"乱倒配置 / 全局 var"模式——参考 cmd/observability.go 注释）。
func Execute() error { return rootCmd.Execute() }
