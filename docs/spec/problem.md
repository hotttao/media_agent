现在的实现有很大问题:
1. hertz_infra 的结构有问题，hertz_infra/observability 现在只是 serverhertz suite 有实现，clientsuite 没有实现么？
2. 我觉得应该创建一个 globalhertz package 专门存放全局唯一的变量，比如 sentinel otelProvider。client 和 server 的具体实现还是放在 serverhertz 和 clienthertz
3. RateLimitConfig 配置应该是多个，一个 server 有多个资源，这个定义就有问题
4. media-cli 的实现有很大问题，我要求的是必须通过生成 client 去请求测试，把 clienthertz 代码测试包含进取，不然怎么能叫 e2e 测试呢，如果你需要增加接口可以在 idl/media_example 下增加一个专门 测试的 service proto