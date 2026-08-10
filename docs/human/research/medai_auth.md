

/home/tao/code/github/agent-platform/control-plane/src/index.ts 是一个 agent-platform 项目控制面的代码。

现在我想把这个项目里与认证鉴权相关代码独立成一个单独的微服务，叫 media-auth。
我已经对这个相关的内容了详细的整理放在 docs/prd/media_auth 目录下，请你review 一下，整体内容是否合理，然后完成以下事情:

1. 使用 skills /domain-modeling 定义 media-auth 项目的领域模型。有什么问题向我提问。
2. 按照 harness/contributing 技术开发规范，和领域驱动设计以及  agent-platform/control-plane/ 的实现，生成 spec 文档。

接下来开发具体功能:
1. 以 domain 为单位，进行开发
2. 开发完之后，需要对比 /home/tao/code/github/agent-platform/control-plane/src/index.ts 中的实现，一方面是 reveiw 代码实现，一方面是复盘 domain 和 workflow 的设计是否正确，特别要注意接口的定义是否一致，需要改成一直，如果确实无法保持一直，说明原因并列出清单，保存到文件中。
3. 复盘 unites 和 项目内部的 e2e 测试
5. 开发 media-cli 完成对 media-auth 接口的 e2e 测试
4. 然后进行下一个 domain 的开发 

数据库改成 pgsql，不用 mysql，参考 /home/tao/code/github/agent-platform/ 中使用的 pgsql 版本。