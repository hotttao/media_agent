# F2 · Skill 内容管理、发布和 Agent 选择

## 完成能力

企业 Skill 资源库接通实际内容：ZIP/tar.gz 上传、Git 仓库/分支/子目录导入、手动同步、文件树和内容增删改、草稿替换/放弃、不可变发布、历史列表和包下载。默认私有，部门 viewer/editor 和企业范围与 Prompt 对称；旧无归属资源可由原 owner 明确归入有效企业。

资源 CAS 与发布内容版本分开。编辑者可改内容、同步既定来源和发布；来源、凭据、ACL、启停和归档由 owner 管理。Git 私有令牌认证加密保存、只写不回显，不进命令参数、目录历史、包或运行快照。Control 运行镜像增加 Git CLI；不新增后台同步、调度或 SRE 服务。

上传和 Git 内容统一为根目录含 SKILL.md 的 ZIP，发布校验入口及 frontmatter。资源选择同时检查当前 ACL、真实企业、Workspace manage、发布版本存在和入口名称冲突。发布或同步不自动更新 Agent；旧选择保留原内容。Agent 安装 ZIP 中脚本时保留所有者执行权限，支持实际运行 Skill 脚本。

## 验证

- Control 全模块、共享资源事实与 Skill archive 单元测试、生成 Skill 客户端及既有 Metadata 集成夹具编译/非 live 检查通过；Control 架构检查通过。
- 真实 PostgreSQL 和 Hertz HTTP 联调同时验证 F1/F2，最终功能包通过（49.984s）。Skill 独立分支通过（15.999s）：实际 ZIP/tar.gz、增删文件、缺失入口拒绝发布、不可变历史下载、草稿放弃、显式配置选择、禁用和权限拒绝。
- 使用本机真实 Git CLI 与临时 smart HTTP 私有仓库验证令牌、分支、子目录、来源 commit 和同步；不访问互联网仓库。验证密文存储、API/历史不泄漏令牌、同步只更新草稿、不自动发运行通知。
- Python 全集：105 passed、12 skipped；跳过外部环境分支未作为通过。相关 Metadata 用例8项通过，新增用例实际执行安装后的脚本。
- Python mypy 143文件、14条 import contracts、vulture、跨包层检查及相关 ruff/format 通过。OpenAPI、Hertz模型/路由/客户端由工具生成；git diff --check 通过。

联调入口见 harness/tools/agent_product_integration/README.md。测试数据库和 Git 仓库按本轮创建范围清理；本机测试 PostgreSQL 暂保留给后续功能检查，不启动 Kubernetes。

## 交付边界

本阶段完成后端和必要 Runtime 文件加载，不表示管理界面或正式模型端到端使用已交付。F3 继续企业 MCP/Plugin，F4 将已交付的 Penpot 原型接入实际界面及 Docker 使用入口。尚未构建最终部署镜像或对互联网私有 Git 服务逐项验收，不以本机 smart HTTP 夹具代表这些部署结果。
