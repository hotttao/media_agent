

/home/tao/code/github/agent-platform/control-plane/src/index.ts 是一个 agent-platform 项目控制面的代码。

现在我想把这个项目里与认证鉴权相关代码独立成一个单独的微服务，叫 media-auth。
我已经对这个相关的内容了详细的整理放在 docs/prd/media_auth 目录下，请你review 一下，整体内容是否合理，然后完成以下事情:

1. 使用 skills /domain-modeling 定义 media-auth 项目的领域模型。有什么问题向我提问。
2. 按照 harness/contributing 技术开发规范，和领域驱动设计以及  agent-platform/control-plane/ 的实现，生成 spec 文档。