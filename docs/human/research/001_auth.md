这个项目有段时间没有更新了，经过这段时间的学习和调研，我决定对整个像个系统实现作如下调整:

## 认证和鉴权

原本: media_auth 自定义实现了完整的用户的认证和鉴权以及 Oauth2 认证等功能
计划:
    1. 引入 ory 体系组件 kratos jwt keto 实现用户的认证和鉴权，oauth2 ory 体系也有组件支持，后续可以扩展，暂时不实现
    2. media_auth 只需要实现 用户信息、组织信息、用户权限、组织权限的增删改查功能。
    3. 用户修改权限，先提交给 media_auth 保存，数据库里通过 is_sync 字段记录是否同步到 ory 组件。实现一个outbox 模式，确保用户权限的同步性。
    4. 所有内部微服务通过 internal jwt 进行统一认证鉴权
    5. 这一套认证鉴权体系，我之前测试过，可以参考 /home/tao/code/github/ddd-learn/deployments/auth/003_keto 里的部署和实现


## workflow

workflow 之前设计是完全自研，由 media_worfklow 子服务实现。 workflow 有很多开源实现，比如 workflow、airflow 等。经过调研，我觉得 conductor 是最满足我需求的选择。

原因有两点:
1. conductor 的设计和实现基本与我之前设想的一致
2. conductor worker 是 pull 模式，跟我预设的将 worker 放置在用户端一致

但是 conductor 跟我的需求还有一层差异。我以下面的示例为例，说明我的需求。

### task 抽象
假如生成一段卖衣服带货视频，需要使用:
1. 使用 即梦 生成图片
2. 使用 豆包 生成视频

即梦生成图片有两种模式:
1. 首帧 + 提示词
2. 多图片 + 提示词，提示词里使用 图一，图二 引用图片

豆包也是类似有两种生成视频的模式。

以上，我可以抽象出 4 个 conductor 中对应的 task。

### workflow 的节点
但是我 workflow 中的节点并不直接对应上面的 task。你可以看我之前的设计。对于每一个虚拟 IP 都有一个对应的 scope。
1. 每个 scope 都对应一个特定的 matrial 范围
2. workflow 中的节点是通过 输入 matrial 和输出 matrial 关联的工作流。

比如上面衣服带货视频的 workflow 有我个节点:
1. 使用`全身图`和`衣服图`生成`模特图` - 使用 即梦生图 task
2. 使用`模特图`和`姿势图`生成`定妆图` - 使用 即梦生图 task
3. 使用`定妆图`和`场景图`生成`首帧图` - 使用 即梦生图 task
4. 使用`首帧图`和`动作描述`生成`视频` - 使用 豆包生视频 task

每一个 workflow 的节点都对应不确定的个数的输入和输出。但是每一个节点都最终要转换成一个 conductor task。

所以我应该如何实现这一层抽象，将我需要的 workflow 转换成 conductor 中的 task。并且我希望的是如果下一次生成，同样的输入已经存在结果，就不要重新生成了，直接使用已有的结果。这一部分，请你帮我调研设计一下方案。

### conductor 环境
conductor 我使用 /home/tao/code/github/infra/conductor/docker-compose.yaml 部署在本地的 docker环境可以直接使用。

## 补充:
1. Organization 对应为公司， Team 是公司里组织
2. 对应关系是，一个公司有多个部门，一个部门可以创建多个 worker space
3. 从需求上讲，我这个网站对后会提供给企业使用，所以需要维护 企业的信息和能使用的功能，企业内部也会有不同的部门。也需要对应的权限控制。
3. 企业的管理员可以在自己的权限范围内，赋予部门权限，默认部门等于企业的权限
4. 共享的范围和具体资源的管理依旧是现在的模式

workflow 节点的单输出是没问题，但是可能输出多张图片，需要用户反馈确定，并且确认是否重新生成。

### 测试

我补充一下测试要求:
1. media_ai 里现在有一个完整的生成视频的 workflow ，我需要你迁移到当前系统上。
2. capability 对应 conductor worker 我希望实现在客户端这一侧，所以我需要你实现一个 media_worker 的工具或者服务，当前的形态我还没有完全确定。但是可以确定的是这是一个 在客户端运行的。
3. 联调测试的时候，我需要你启动 media_worker，在迁移 media_ai 的数据时候，真实启动 workflow，只不过 media_worker 会根据确定的任务直接 mock 返回 media_ai 里已经有的结果。这样就可以完整测试，workflow 的执行流程。
4. 测试的时候，先只执行一个完整流程。我确认没有问题之后，再迁移所有数据。