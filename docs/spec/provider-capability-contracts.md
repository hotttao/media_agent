# Provider Capability契约纠偏

状态：2026-10-06依据001_auth.md四类原始任务恢复契约并进入实现；generation_method术语已采纳。豆包第二模式按“类似两种”定义项目多图视频契约，本轮仅mock，不宣称其原生API支持已验证。尚未部署或完成全部纠偏。

## 范围与边界

修正media_workflow能力注册/模板校验/编译、media_worker能力处理器与fixture，以及相关CLI/HTTP测试。Render/Finalize/审批、Gateway身份、Personal/Managed隔离与既有Outbox机制保留。UI、历史全量迁移、Conductor启动幂等及真实Provider HTTP适配器接入不在本轮自动扩展范围。

## 能力与版本

平台字段表达Provider，现存数据库provider列继续存放platform，不重复维护另一份平台值。generation_method为t2i/i2i/t2v/i2v；generation_type为具体输入模式。业务用途、模特图分类、Worker实例、API/浏览器调用方式都不进入核心身份。

| platform | generation_method | generation_type | Capability Ref | images数量 | 输出 |
|:---|:---|:---|:---|:---|:---|
| jimeng | i2i | first_frame_prompt | jimeng.i2i.first_frame_prompt@1 | 1 | IMAGE |
| jimeng | i2i | multi_image_prompt | jimeng.i2i.multi_image_prompt@1 | ≥1、有序 | IMAGE |
| doubao | i2v | first_frame_prompt | doubao.i2v.first_frame_prompt@1 | 1 | VIDEO |
| doubao | i2v | multi_image_prompt | doubao.i2v.multi_image_prompt@1 | ≥1、有序 | VIDEO |

Personal Key固定platform.generation_method.generation_type；Managed变体按已有独立Key规则增加.managed，不改变已发布Target。主键(Key,version)，新契约另有(provider,generation_method,generation_type,execution_target,version)部分唯一索引，并验证Key与字段一致。

输入为prompt:string（必填）和images:resource_ref_list（必填），MediaType=IMAGE。first_frame_prompt的min_items/max_items均1；multi_image_prompt的min_items=1、max_items=0表示不限制数量。项目通用输出为至少一个candidates元素，包含index、受控媒体引用/url、可选content及provenance；媒体形态由Capability OutputMediaType约束，接受后业务输出仍单一。不能把这些项目协议字段直接视为Provider原生HTTP请求。

Capability输出`output_media_type=IMAGE/VIDEO/TEXT`与Template的`output_asset_kind`解耦。一个IMAGE能力可以服务model_image/style_image/first_frame，只要Materials注册类型与技术输出兼容。输入schema可声明媒体类型限制，校验不能从参数或业务Step名字猜测。

## P1持久化与兼容

migration023增加nullable output_media_type/generation_method/generation_type；三字段均null仅兼容旧output_asset_kind非空记录。新Provider记录三字段均必填且旧output_asset_kind为空；provider须规范小写平台标识，不能为client。校验i2i/t2i输出IMAGE、i2v/t2v输出VIDEO，首帧/多图必须i2i/i2v；Key与结构化身份一致。不回填、不改旧种子迁移、不修改既有能力、模板版本、IR、Definition、Job或产物。插入四个唯一新Key/TaskDefinition，published契约不可更新或删除、不能原位转换旧契约；有新Provider记录时降级拒绝。

手写DAL model和领域Capability增加GenerationMethod/GenerationType/OutputMediaType；Input增加可选MediaType、MinItems/MaxItems。数量约束仅适用于资源数组，非负且非零MaxItems不得小于MinItems。Repo验证新契约固定prompt/images、媒体及数量模式和规范Key；旧null不推断模式或输出形态。旧能力和冻结IR保持原语义。

## P2模板与编译

模板的业务分类继续保留；通过Materials AssetKind元数据校验与技术媒体类型兼容，而非名称相等。固定能力参数由Invocation Mapping提供prompt与有序images[]。新Provider能力和新模板/Workflow版本用于新样本，不自动选择Provider，不按Step名称隐式映射。Semantic IR冻结Provider技术类型和资源限制；老版本IR不改hash，Compiler兼容已支持旧版本，不直接重写旧定义。

Template Create/Update/Publish对新契约校验输出AssetKind的resource_type=ASSET、状态SYSTEM/ENABLED及media_type；旧契约仍按原output_asset_kind相等规则处理，不猜技术类型。Workflow发布在本地事务前通过AssetKindLookup读取输出与资源slot元数据，缺失、禁用、媒体未知或resource_type不一致时拒绝编译。TEXT slot不当成媒体引用。

PRODUCT/VIRTUAL_IP是独立聚合输入，不查询同名AssetKind：其asset_ref分别为已授权选定的商品图和Virtual IP全身图，技术类型为IMAGE。实际对象的企业归属、MIME/大小仍在交付时验证，不能仅凭聚合类型接受任意URL。MATERIAL/ASSET slot继续依据注册元数据，不按节点名称猜类型。

Semantic Compiler对Invocation Mapping检查必填参数、表达式类型、有序list的固定长度与每个asset_ref slot的媒体类型；首帧模式不能映射两张图片。编译后的CapabilityIR仅对新类型契约保存provider、generation_method、generation_type、output_media_type及inputs（含media_type/min_items/max_items），拷贝输入schema避免与可变快照共享切片。新增字段均omitempty，保持schema_version=3的兼容扩展；旧契约不写这些字段，即使原provider=client也不增加字节。业务output_asset_kind与原审批/Render/Finalize编译保持独立。实际资源MIME与运行时数量校验由P3补齐，不能用静态slot元数据替代。

## P3 Worker与网关

Personal Worker按Provider Capability注册处理器，共享客户端poll/heartbeat/result协议；Managed Runtime继续按冻结Target隔离，不新增托管Provider。网关按声明schema解析受控ImageRef并验证MIME/数量，不硬编码resources字段。Mock能力入口与生产契约一致，结果在fixture内按样本Job/Step映射，以支持多个模板复用同一Capability，不能在生产处理器里猜业务步骤。

客户端测试profile为`provider_fixture`，私有配置由`MEDIA_WORKER_FIXTURE_CONFIG`指定：顶层tasks数组，元素为job_id/step_key/capability_key/capability_version/result_urls。精确Job/Step唯一、规范支持的四类v1契约、每个结果1–8个不同存储对象URL；配置上限1MiB与256条绑定，未知字段/尾随数据拒绝且错误不包含URL。仅注册配置用到的契约，CLI allowlist只能缩小。配置启动前加载并拷贝，不支持运行期变更；旧media_ai_fixture隔离保留且新profile拒绝旧业务覆盖参数。客户端只检查已渲染prompt、images格式/数量，不渲染提示词，不请求任意输入URL判断MIME；实际存储媒体校验在服务端完成。当前poll仍按Capability队列领取，精确Job绑定不等于Job队列隔离；单样本启动前必须审计待处理队列。

Materials对象解析响应兼容增加content_type、size_bytes，来自S3已有HeadObject结果，不根据扩展名推断；URL仍短期签名且不持久化。新Provider声明媒体限制的参数只接受企业Object Path或配置StorageOrigin下本企业对象URL，拒绝任意外部URL。Gateway解析时校验对象非空、签名有效期及存储MIME与声明MediaType一致，参数数量/类型/契约引用不符不创建Dispatch；重交付重新校验并签名，不能交付旧过期URL。旧契约不新增媒体限制。这里只验证受控上传对象存储的MIME元数据，不表示每次poll下载全量媒体重新检测字节。

## P4样本与验收

样本脚本接受显式Provider能力配置，不擅自把所有生图模板选为某个Provider；沿用已对齐示例可选择即梦多图生图和豆包首帧生视频。旧能力/执行留存且可读；新样本走API创建新模板和Workflow版本后执行，不改历史记录。

`media_ai_workflow_migrate`增加`--image-capability`/`--video-capability`（或MEDIA_SAMPLE_IMAGE_CAPABILITY/MEDIA_SAMPLE_VIDEO_CAPABILITY），两项必须同时提供；图片模板映射两张输入，要求Personal i2i/multi_image_prompt，视频模板映射单张首帧，要求Personal i2v。参数包含显式版本；模板名增加引用后缀，capability_version和platform/gen_way/gen_type由该引用规范填写，Invocation使用images而非旧resources。两项均未提供时仅保留旧fixture兼容，不默认为任何Provider。

公开HTTP门禁在media-cli/e2e：FixtureConfig只对已通过CLI提交的Job创建0600私有结果配置、校验契约/队列/对象，不启动Worker。Execution观察同一Job，用HTTP请求重新生成和重放、独立CLI重试及确认；Worker须在外部显式启动。继续观察模式不重发原决定，不启动替代实例。验收遇到的可靠Retry交付窗口与人工测试任务回收须保留失败及恢复两组证据，不能将受控恢复成功包装为自动故障恢复成功。

门禁包括域/Repo/迁移、模板共用同一生图能力但分类不同、IMAGE/VIDEO不兼容拒绝、参数顺序、图像媒体与首帧数量限制、Personal/Managed隔离、Mock能力注册、真实网关与独立CLI的一条完整生成/审批/重新生成/Retry流程。Provider只mock，其他边界使用真实服务；不能把已有业务型能力样本冒充Provider纠偏成功。
