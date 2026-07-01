我觉得目前使用 media-cli e2e 的流程还有很大问题:
1. 首先在启动测试前，系统没有预设置环境变量，比如 APP_ENV 为 local
2. 需要哪些基础服务，希望提供一个脚本去启动，并提供探测服务是否已经正常
3. 带测试服务，也需要脚本来管理启动，和重启，避免每次测试，服务没有重启，而且要能探测服务确定已经重启。
4. 测试完成后，需要能够清理所有服务

这个类似go test 里面 tearup teardown 的逻辑。测试的时候，我需要执行 tearup 初始化环境
中间执行多个测试，测试执行完成之后，执行 teardown 清理所有服务。
如果某一个带测试服务，修复了bug，可以使用脚本重启该服务。
另外，我希望对资源的管理抽象成模块，他们tearup teardown 是对等出现的。整体环境的 tearup teardown 的核心入口能清晰看出来资源编排过程。请你设计实现。实现后，请更新 harness/contributing/e2e.md，
最终测试流程如下：
1. tearup 基础服务
2. tearup 被测试服务
3. 执行 media-cli 具体的测试命令，测试被测试服务
4. teardown 被测试服务

基础服务可以复用，除非确定不再测试了，在手动清理。

-------------

实现后，我发现一个问题，使用 media-cli e2e 测试时，这种增加命令的来管理服务的方式，跟直接使用 docker-compose 一样，甚至没有 docker-compsoe 灵活。所以我觉得不需要加了。保留每个服务的 dockercompose，每次测试也是通过启动 docker-compose 来管理服务。但是不需要在 media_cli 中增加命令。

另外我其实需要一个东西:
1. 每个服务下面的 conf 都有很多配置文件，他们是根据 APP_ENV 来选择的。
2. 另外每个服务的 docker-compose 都是写死了 环境变量，但是我希望的是他们能引用相同的 env file，我通过切换 env file 来切换不同的infra。
请重新规划实现，删除不需要的代码，并更新 harness/contributing/e2e.md

