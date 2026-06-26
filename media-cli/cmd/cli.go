// cli.go — media-cli 进程级共享的全局 flag 值。
//
// 历史说明：本文件原本计划在 cmd/internal/cli/，因 cmd ↔ subpackage 不存在 import cycle
// （cmd 不 import subpackage，subpackage 通过 main.go 的 blank import 注册），
// 不需要隔离层；内联回 cmd 根，subpackage 直接 import cmd 即可访问。

package cmd

// FlagHost 全局下游服务地址，各子命令可读取。
// rootCmd init() 把它绑定到 rootCmd 的 --host persistent flag。
var FlagHost string

// FlagVerbose 详细输出。
// rootCmd init() 把它绑定到 rootCmd 的 -v / --verbose persistent flag。
var FlagVerbose bool
