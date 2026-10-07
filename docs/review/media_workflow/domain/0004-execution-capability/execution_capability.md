# Execution Capability 实现一致性审查

审查基线：`docs/human/research/001_auth.md`（含初始提交252eb2e的task抽象）、`002_explain.md`及当前代码。2026-10-06重新审查，只记录差异，不把未确认的契约写成已实现。

## 结论

Capability持久层、版本化引用、Semantic/Backend Compiler和Worker Gateway已经存在；旧报告中“没有聚合/表/仓储”的结论已失效。但Provider契约的语义及完整能力清单仍为missing，不能用端到端mock流程成功证明该抽象正确。

原始需求明确列出即梦生图的“首帧+提示词”“多图片+提示词”两种模式，豆包生视频也有两种模式，并明确合计四个底层task。随后使用Capability这个名称应保留原Task的Provider操作语义，避免与Conductor内部Task混淆，不是重新按业务步骤划分。

原文未正式定义platform/gen_way/gen_type字段的枚举和命名。可确认的维度是平台、生成图片/视频、具体输入模式；不能未经对齐就把生成类型解释成业务Asset Kind，或用t2i/i2i等新分类替代原首帧/多图模式。豆包第二种视频模式仅描述为“类似”，精确输入约束需要展开，不能自行认定为首尾帧或任意多参考图协议。

## 影响

- migration008和fixture按media.image.model/style/first_frame及media.video.generate注册能力、provider=client，混入业务用途，偏离原始Provider任务抽象。
- Template.validateCapability与Semantic Compiler要求能力output_asset_kind等于模板业务分类，使同一Provider生图契约无法供不同业务输出分类的Template复用。
- mock按业务型Capability Key选择样本，原文要求的四类Provider模式没有完整对齐和验证。
- 2026-10-06新拟的migration023只列豆包多图生图、即梦多图生图、豆包首帧生视频三种能力，尚未部署；它没有完整覆盖原文四类，不能作为定稿目录。新增类型字段不替代平台/生成方式/生成类型的先行对齐。

## 能力覆盖矩阵

| 原始能力/职责 | 所有者 | 状态 | 证据/差异 |
|:---|:---|:---|:---|
| 即梦首帧+提示词生图 | execution_capability / Personal Worker | missing | 当前已发布种子未按该Provider模式注册 |
| 即梦多图+提示词生图 | execution_capability / Personal Worker | missing | 002示例已定义，当前业务型fixture没有证明该契约实现 |
| 豆包两种模式生视频 | execution_capability / Personal Worker | missing | 首帧示例已有；第二模式的准确参数/约束未展开，两模式均需按契约验证 |
| 模特图/定妆图/首帧业务槽位与提示词 | Template / Invocation Mapping | covered | 原始节点语义保留，但当前能力引用错误 |
| 固定版本编译与网关派发 | conductor_compilation / worker_gateway | covered | 已有IR、Definition、Capability版本映射；需接正确目录 |
| 真实模型原生调用 | 客户端Provider适配器 | external-owner | 本轮按原测试要求只mock，不扩大真实模型接入 |

## 迁移要求

当前进展（2026-10-06）：恢复原始四类目录为jimeng.i2i.first_frame_prompt/multi_image_prompt和doubao.i2v.first_frame_prompt/multi_image_prompt。规范Key/字段一致、输入数量/媒体限制、版本化/冻结及migration023已在domain/Repo/race和真实隔离22→23数据库验证，旧记录指纹不变，未部署。先前三条种子草稿已修订；Provider契约持久化已补齐，Template/IR/Worker端到端映射仍missing，因此整体仍未完成。豆包多图视频定义为项目mock契约，真实Provider原生支持仍需未来适配验证。

2026-10-06用户采纳Generation Method生成方式术语；统一字段为generation_method，使用t2i/i2i/t2v/i2v等输入输出媒体转换含义。生成类型的准确枚举仍待对齐；首帧/多图输入模式必须在契约中明确区分，不能仅凭它们都属于i2i就合并成同一个固定任务。原文豆包第二视频模式的精确约束仍未展开，不据术语采纳推断已确认完整目录。

先确认平台、生成方式、生成类型/输入模式的正交含义与完整初始目录，然后固定key/version、输入输出schema及Conductor TaskDefinition。模特图、定妆图和首帧可以通过各自Template复用相同底层模式。旧字段与既有发布快照只作兼容，不原位改写；未部署的草案须按确认后的定义修订，不以三个新Key覆盖四类原始任务。当前只做review，纠偏实施等待契约对齐。

## 2026-10-06 模板与编译接入进展

Template输出分类校验已区分新契约技术媒体与旧契约业务分类；Publication在事务外读取Materials元数据并传递到Semantic Compiler。新IR冻结Provider身份/类型与输入媒体/数量schema，旧IR和旧provider=client不增加新字段。相关测试覆盖多业务模板复用、媒体/数量拒绝、元数据失效、图片顺序和旧规范JSON兼容；Workflow全包与相关race/架构门禁通过。

客户端Provider处理器、Gateway实际资源MIME/数量验证、新样本部署与HTTP/CLI完整执行仍为missing；不能将本阶段标记为整体纠偏完成。

## 2026-10-06 客户端与交付代码进展

后续已实现独立`provider_fixture`，复用已有客户端协议，支持四类底层契约并按精确Job/Step配置mock产物。Gateway按发布契约校验输入类型/数量和Capability引用，Personal/Managed新领取与重交付均覆盖；Materials兼容增加HeadObject媒体/大小元数据，使交付时校验受控企业对象、实际存储MIME、非空与签名有效期。新协议代码及IDL/Swagger已更新，旧fixture隔离保留。

Worker全包race、Workflow/Materials全包、Gateway/下游client/Storage/handler关键race与两服务架构检查通过。只有代码与单元门禁完成：运行服务未部署新协议，新样本脚本与公开HTTP/独立CLI的完整Provider流程仍missing，不宣称整体已可用；真实模型适配仍external-owner。

## 2026-10-06 部署与受控单样本契约验收

已部署新Materials/API及migration023；新脚本显式选择即梦多图与豆包首帧，模板名隔离旧版本。唯一Job在受控回收一条未交付任务后四节点成功，真实IR/Conductor/Inbox证明前三个业务分类复用同一即梦能力，输入均prompt/images，客户端mock回写并由服务端Render/Finalize；媒体读取3 PNG+1 MP4通过。契约接入为covered，真实模型external-owner。

首次自动Retry门禁失败不能删除或改写为通过；交付窗口仍归原Workflow可靠Retry未完成任务。具体部署、失败和同Job恢复证据见[Provider验收记录](../../workflow/0003-conductor-compilation/provider-capability-acceptance.md)。没有关闭整体Goal或新增UI/历史/Agent工作。

2026-10-07已部署精确延迟交付修复，并以新的唯一回归Job通过无人工干预HTTP/独立CLI四节点门禁及媒体读取；契约联调covered，原失败记录不删除。完整持久Retry与崩溃恢复仍missing；不能把窗口补偿或正常单流程成功当作整体可靠性完成。证据追加在同一Provider验收报告。
