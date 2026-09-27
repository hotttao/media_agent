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