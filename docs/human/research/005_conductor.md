# conductor 任务拉取和结果回写

正常逻辑下:
1. media_workflow 和 conductor 都运行在服务器一侧
2. media_worker 有可能运行在客户端一侧，也有可能运行在服务器一侧。无论哪种方式，都设计到对 media_worker 的认证和鉴权问题。

所以目前想到的方案是:
1. conductor 的任务拉取和结果写回都由 media_workflow 代理。这个具体需要你调研后跟我确认，我目前不太清楚 conductor 自己是如何拉取和回写任务的如何认证和鉴权的
2. 在整个认证鉴权流程下，增加通过 api token 认证的方式。认证的流程参考 /home/tao/code/github/ddd-learn/docs/human/auth 下两个以 005_ 开头的文件。这个流程更复杂一点，这个里面增加了一层 agent 权限。但是我认为在当前的情况下不需要，可以认为 agent 与 用户拥有同样的权限，增加 agent 的身份核心只为了审计
3. media_worker 如果运行在客户端一侧，客户首先需要再网站申请一个 api token，然后在 media_worker 中配置这个 token，用这个 token 来认证和鉴权。客户只能拉取和提交自己的任务。
4. media_worker 如果运行在服务端一侧，服务端保存的任务应该有任务归属的用户，media_worker 用任务记录的用户进行认证和鉴权，甚至后面可能还有限额控制。

当下还有一个问题。现在基础服务的管理比较混乱。后面做一个统一:
1. 所有的基础服务都在 /home/tao/code/github/infra 管理，并以服务常见端口暴露在宿主机上上
2. media_agent 使用自己的容器网络，只有网管服务和 react 测试服务对外暴露端口

## api token
目前位置我觉得，关于如何实现 api token，还没有对齐。api token 的认证架构，你可以参考 /home/tao/code/github/ddd-learn/deployments/auth/005_ory_token 下的实现。其中:
1. ory talos: 开源实现，只支持 sqlite3，所以这个服务只能单节点部署。我们先使用 ory 体系完成功能，后续再考虑如何替换
2. token-manager: 不用创建新的服务，直接实现在 media_auth 服务内
3. agent_gateway: 请求 media_auth 服务，为 agent 申请 api_token

## media_worker 的实现问题
我之前其实已经规划了 media_worker 的实现，对应的是当前目录的 media_gen 服务。我需要你协助我做一个判断:

我目前还不能确定 media_worker 最终的服务形态，有可能是 cmd 有可能是 http server。有可能是 http server 的原因是:
1. 当前 worker 是 pull 任务的状态，woker 是直接部署的在客户端一侧，在服务端一侧是不太好控制具体 job 执行的优先级
2. worker 如果实现为 http server 可以提供简单的管理页面，用户可以配置自动拉取并执行所有 job，也可以查看有哪些 job，通过页面点击的方式执行某一个具体的 job

你觉得我这样的需求合理么，如果合理，需要你复用 media_gen 服务的代码，按照 ddd 领域驱动的方式来开发。不过这里面的代码有过期代码，需要删除。然后迁移 media_worker 的代码。最终保留 media_worker 作为服务名称。请注意代码要严格按照 harness 约束的代码规范，使用 ddd 领域驱动进行开发。
