/home/tao/code/github/media_ai 是我之前的一个 idea，希望实现一个基于提示词模板的 workflow，详细设计参考 /home/tao/code/github/media_ai/media-materials/docs 下的 workflow.md  workflow-template-execution.md 文件。我本来的设计是分成两个服务，matrials 是素材库，管理所有提示词使用的素材，模板和workflow。scheduler 负责调度 workflow，tempalte 的执行，一次template 执行是一个 job。但是这个版本的逻辑里面，我发现这两个服务都需要 workflow 的校验逻辑。两边都要冗余 job 的输入和输出。

所以这个版本，我决定重构一下:
1. matrials 只负责素材管理，每日发布计划
2. workflow 替代 scheduler，包括 tempalte，workflow 的存储和执行

在原有的版本上，我需要增加一下内容:
1. 我需要给 workflow 增加一个 scope 的概念，每一个 asset_kind 都属于一个 scope，表示素材所属于业务范围。比如 AI 服装是一个业务，AI 国学是一个业务，template 也有对应的 scope。teamplate 和workflow 只能使用对应 scope 的素材。
2. template 现在 inpute 和 output 为了适配所有资源现在，没有明确定义所有 resource_type 的输入，这个要在这个版本优化，input 改成一个列表，里面每个元素是一个 resource_type 的输入。

权限相关:
workflow 的相关的业务会通过 media_auth 进行认证和鉴权，media_auth 定义了 user/team/workspace，他们与当前业务的关系是:

1. workspace: 会关联一个 scope，表示当前 workspace 属于什么业务
2. matrials 里的 virtual_ip 也会关联一个 scope，表示当前 virtual_ip 属于什么业务
3. 一个user 可以有多个 virtual_ip
4. product 也有对应 scope，表示当前 product 属于什么业务，但是没有 user 概念，只要 workspace 是一样的 scope 就可以使用 product 的素材。

user 可见的 virtual_ip 符合 docs/research/permission.md 中的权限规则。

现在业务规则变了比较多，需要梳理业务规则。请使用 domain-modeling 这个 skills 来梳理业务规则。有不明确的地方，向我提问。

补充一点，我希望 workflow 在执行 job 的时候，input 输入参数是自闭环的，不需要在向 matrials 里查询。

能注册 asset_kind 之后，为了让 job 的输入参数是自闭环的，就需要更改 input 的定义。原先是  [{}]，要改成 {
    "product": {}, # 可谓 null
    "virtual_ip": {}, # 可谓 null
    "asset": [{},],
    "matrial": [{},],
}

请你用 第一性原理，审查核心业务是否闭环/是否简洁/是否足够解决用户问题。确认没问题后，进入开发阶段。