现在的实现有很大问题:
1. hertz_infra 的结构有问题，hertz_infra/observability 现在只是 serverhertz suite 有实现，clientsuite 没有实现么？
2. 我觉得应该创建一个 globalhertz package 专门存放全局唯一的变量，比如 sentinel otelProvider。client 和 server 的具体实现还是放在 serverhertz 和 clienthertz
3. RateLimitConfig 配置应该是多个，一个 server 有多个资源，这个定义就有问题
4. media-cli 的实现有很大问题，我要求的是必须通过生成 client 去请求测试，把 clienthertz 代码测试包含进取，不然怎么能叫 e2e 测试呢，如果你需要增加接口可以在 idl/media_example 下增加一个专门 测试的 service proto
5. clientsuite 的实现 ForApp 改一个名字，语义不明

服务注册与发现: 
1. 文档: https://www.cloudwego.io/zh/docs/hertz/tutorials/third-party/service_discovery/consul/

限流和熔断的配置不完整，参考:
1. https://sentinelguard.io/zh-cn/docs/golang/flow-control.html
2. https://sentinelguard.io/zh-cn/docs/golang/circuit-breaking.html

完善后，把限流和熔断的配置增加到 consul 配置中心中，测试从配置中心拉取配置。

cli:
2. 约定一下配置文件，env=local 是本地测试配置，所有组件都启用，media-cli 也增加一份配置，配置加载逻辑同 media-example，也是根据 env 加载不同的配置
3. dev 配置文件，只打开可观测组件，其他组件都关闭
4. media_example 增加一层子命令，app 为一级子命令 service 为二级子命令。目录也按照这个层级组织。