// media-cli 是 media_agent 的命令行工具，基于 spf13/cobra。
//
// 定位：
// - 供用户手动调用 media_agent 各微服务暴露的 API；
// - 同时作为 e2e 测试工具。
//
// 实现基于 hertz_gen 生成的 typed client 代码（见各 hertz_gen/<service>/ 包）。
// CLI 属于基础设施/测试工具层，可直接 import hertz_gen 客户端 stub，
// 不受业务层「禁止 import hertz_gen」约束（该约束针对 service/workflow/policy/domain）。
package main

import "media_agent/media_cli/cmd"

func main() {
	cmd.Execute()
}
