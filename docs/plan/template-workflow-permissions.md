# Template / Workflow 权限落地

依据[spec](../spec/template-workflow-permissions.md)，不包含UI/历史全量/Job权限扩展。

- [x] P1：补齐Template/Workflow公开入口、真实caller、viewer/editor/owner策略及负向测试；Workflow无公开Delete路由，但内部Delete用例也校验owner。
- [x] P2：企业/Workspace归属、当前企业与部门权威查询、同企业Grant/public、IDL/迁移/生成器与安全旧记录兼容。
- [x] P3：权限前置分页、事务锁/CAS、模板发布版本事实、Workflow引用与发布/执行隔离；跨服务撤权窗口不冒充原子事务，见spec。
- [x] P4：全量/race/架构、隔离PG、运行镜像/HTTP与独立CLI回归；不创建新的完整模型流程。仅本轮权限与既有完成样本门禁，不代表旧Materials Bindings接口已修复。
- [x] P5：补齐Update/Publish功能准入门禁，owner/editor不能绕过功能撤销；多账号真实HTTP/CLI、最后部署与提交，不触碰既有用户部门功能。

当前能力矩阵：Template 10个IDL入口（Create/Update/Get/GetVersion/List/Archive/Publish/Grant/Revoke/Visibility）、Workflow 10个IDL入口（Create/Update/Get/List/Archive/Publish/Validate/Grant/Revoke/Visibility）、内部Delete与Submit权限接线covered。纯Renderer与执行冻结快照covered，不额外新增公开Render/Parse接口；旧文档把尚不存在的HTTP端点列成既有能力，已更正（deprecated-with-reason：删除虚构路由描述，而非删除业务代码）。企业/部门成员与功能事实external-owner media_auth，Workspace准入external-owner media_control，DAG/retry/审批调度external-owner Conductor。部署多账号测试不是所有矩阵项的证据，范围如后文说明。

## 当前证据

P1/P2/P3核心策略与handler race通过：陌生人/无caller写入、伪造granted_by、viewer/editor不成为owner、跨企业public/Grant、退出部门后拒绝陈旧caller团队列表、authority故障、不绑定旧public，以及Workspace/企业越界。版本读取仍遵守资源ACL，旧记录只owner兼容。当前端口没有开发UI、账号签发或历史迁移。

Workflow全量race、构建与架构检查通过，Auth相关team/internal handler race与架构通过。IDL DTO由hz-model生成，OpenAPI由swagger-gen生成；迁移先在独立数据库执行，再通过gorm-gen生成查询。生成器因历史step_bindings文件名遗留导致首轮重复定义编译失败，生成器增加严格校验的旧生成文件清理后重跑通过，不手改查询。首轮错误映射增加了未使用import、CLI门禁初版数字工具未定义，两轮编译失败均保留，修正后通过，不算首轮PASS。

隔离PG部门权限/分页37.08s PASS；绑定企业/Workspace的分页/计数、撤权后无写入、旧published/新draft版本分离、发布头和版本原子落库、禁止企业重绑定28.99s PASS。隔离IAM事实为明确端口，不能冒充部署多账号角色验收。

公开HTTP与独立CLI51.54s PASS：仅从owner旧源复制4份Template和一个新绑定Workflow，创建/发布/Grant/visibility/Revoke、匿名401/403、跨企业创建403、独立CLI发布/查询至registered。测试Workflow `wf_3ecca345-a18c-4178-826e-6163d8a88079`，4份模板及Workflow均已归档，数据库确认Job新增0；没有模型Worker或新完整流程。多账号viewer/editor矩阵由service/handler/隔离PG证明，不把owner HTTP门禁说成已用真实第二账号运行。

已部署Auth `sha256:d9674dbb27afbe51283a33e36d03200c752c9ffb96a93150eb36195954bbd0ba`，Workflow API `sha256:18001cc48b85b77c100221bd4e71d99da6c11d87938f5f7fb456bce8483be549`，内部Worker `sha256:57369cffe3b24fca13540e7a56772059c77a042fd52e159bf18ba9b51ec7b76b`。Auth构建首次因Dockerfile复制已被dockerignore排除的.env失败，移除镜像凭据复制、改Compose运行时env_file注入原配置后构建/启动正常；auth/me200。不启动可观测性、不重启Conductor。业务库25/dirty=false，5173 HTTP200。已有完成样本四产物完整SHA256复读4.87s、公开旧任务路由410复验1.17s通过，未修改该样本或素材。

宽范围CLI e2e在部署门禁环境中运行30.144s失败，不能标全量PASS：旧 `TestMaterialsBindingsEndAt` 调用 `/materials/{id}/bindings` 返回404（不属于本轮权限实现，不扩展去开发该接口）；`TestWorkflowCompletedOutputsProbeContract` 的本地HTTP替身被外部file-results环境变量污染，补显式关闭其文件模式。前者创建的唯一VIP/Material由原测试defer软删，另核对清理；没有改用户旧数据。后续默认环境的单测与这批显式公开门禁分别记录，不用默认Skip代替旧绑定功能验收。

## 2026-10-08 修复与最终验收

Update/Publish 增加独立功能准入：Template 必须在其精确企业拥有当前 workflow 功能；Workflow 必须能在其精确 Workspace 使用 workflow。owner/editor 不绕过功能许可，别的部门可用不等于当前 Workspace 可用。授权服务故障 fail-closed 503；读、归档和撤权不因功能停用而被捆绑禁止。旧无绑定记录仍不猜测归属，但生产 authoring 需要当前可用 workflow 功能。真实多用户门禁还发现 Workflow 可见性/撤权的 owner 拒绝曾返回普通错误，被 handler 映射为400，已统一为领域 Forbidden/403，并补回归。

最终 Workflow/Auth `go test -race ./...` PASS；CLI `go test -race ./cmd/... ./e2e` 默认环境 PASS（外部门禁显式Skip，不冒充部署证据）；两服务架构检查 PASS。隔离PG race三项 PASS：撤权赢得editor写锁63.03s，企业/Workspace分页与版本发布持久化33.21s，部门权限/分页29.03s；总136.101s。夹具权威端口明确提供当前功能事实，不用隔离桩代替多账号HTTP验收。

`TestResourceMultiuserGateway` 最终69.87s PASS（含清理）：本地Kratos新建3个临时身份，真实浏览器密码登录，经网关申请各自 User API Token；在新部门/Workspace内验证viewer读但不能编辑，editor可编辑但不能发布/授权/撤权/改可见性，企业admin不是resource owner；停用新Workspace部门workflow功能后owner/editor写入及发布403，读仍可用，恢复后成功；Grant撤销后立即403；独立CLI viewer读取、editor发布拒绝、owner发布至registered。最终夹具Workspace `91fdbfb5-b62b-4f71-8d7c-bac32983278e`。未提交Job/模型调用，不改旧部门或企业功能。定义归档、部门停用、临时Token撤销、临时企业成员移除、Kratos身份删除；Workspace和业务映射/审计保留。

保留失败记录：初轮误把企业成员POST的200预期为201；第二轮发现上述400映射；中断导致一轮一小时临时Bearer过期，权限断言成功但撤销清理401，不能算该轮整体PASS。清理改用本轮自身浏览器身份，最终完整通过。中断轮3个已经过期的测试Key，以已核对的精确ID调用Talos Admin撤销，并GET确认全部REVOKED，随后仅修复这3行本地凭据元数据为revoked；无既有Key/用户状态或业务数据改动。这是测试运维恢复，不是新增公开绕过鉴权接口。

最新运行镜像：Workflow API `sha256:3f7e46e90c947a3996ddeac1536b2106951fd2e4cc78683372fd7a4a4de5bbb4`，内部Worker `sha256:37281255023292457467dd1e42ea82115f957a78241aabdaafa83719fdd0188f`；Auth仍为前述版本。两进程restart=0，业务库25/dirty=false，Vite5173 HTTP200。已有完成样本四产物Range206/完整SHA256复读15.05s PASS，旧任务路由410门禁1.31s PASS。本次所有测试Workflow新增Job=0。服务保持运行，未启用观测后端、未修改Conductor或UI。
