我觉得目前使用 media-cli e2e 的流程还有很大问题:
1. 首先在启动测试前，系统没有预设置环境变量，比如 APP_ENV 为 local
2. 需要哪些基础服务，希望提供一个脚本去启动，并提供探测服务是否已经正常
3. 带测试服务，也需要脚本来管理启动，和重启，避免每次测试，服务没有重启，而且要能探测服务确定已经重启。
4. 测试完成后，需要能够清理所有服务

这个类似go test 里面 tearup teardown 的逻辑。测试的时候，我需要执行 tearup 初始化环境
中间执行多个测试，测试执行完成之后，执行 teardown 清理所有服务。
如果某一个带测试服务，修复了bug，可以使用脚本重启该服务。
另外，我希望对资源的管理抽象成模块，他们tearup teardown 是对等出现的。整体环境的 tearup teardown 的核心入口能清晰看出来资源编排过程。请你设计实现。实现后，请更新 harness/contributing/e2e.md


然后我还需一个工具，放在 harness/tools/new_server 目录下，用于从 media_example 初始化一个新项目。

现在 media_example 和 media_auth 的数据库配置还是 mysql，需要统一改成 postgres 而且我希望 mysql 的连接参数，可以分成 USER/PASSWORD/HOST:PORT/DATABASE 这些独立的参数，需要注意特殊字符的处理。