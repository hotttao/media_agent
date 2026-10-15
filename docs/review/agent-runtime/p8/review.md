# P8 · Docker 原生链路联调

日期：2026-10-15。状态：完成，按阶段生成审查记录并提交。

## 运行环境调整

用户明确后续功能先在 Docker 环境测试，关闭本机 k3s。已停止 k3s 服务，并终止属于 `/var/lib/rancher/k3s/data/` containerd-shim 的工作负载树：59 个 shim、290 个进程；核查为 `inactive/dead`、残留 shim 为0。未删除持久卷数据。操作前后本机内存使用约19GiB→13GiB，可用内存约11GiB→18GiB。

Kubernetes 双 Ready/RWX 实测按用户授权延期到独立的可用存储环境，不阻塞后续 Docker 功能开发；没有把延期记成实测通过。

## 完成内容

- 新增 Docker-native 集成测试，实际走 Runtime Controller HTTP handler/生成 SDK → Docker Engine 创建容器 → Gateway 原生身份查询/登记 → Agent 签名 HTTP/Session 服务/数据库/事件总线。
- 单 Workspace 单 Agent。验证创建幂等、真实 bootstrap proof、健康与登记、重复 Turn 幂等、Session FIFO、同一 Gateway 两个 SSE、持久事件回放。
- 实际重启容器，校验 container UID 保持、native generation/incarnation 更新、空闲 Session execution_epoch 变为2、固定 context version/native handle 保持、持久文件保留、旧 incarnation 拒绝写入。
- Docker scale 仍返回501，无新增扩缩容实现。
- SSE 夹具在 HTTP 服务关闭时取消后台轮询，在删除 Session/关闭数据库前关闭服务，收拢测试生命周期。
- 新增一键脚本、受限资源 Compose 和协议镜像构建入口；依赖按 uv.lock 同步，不调用真实付费模型。

## 资源与清理

PostgreSQL 上限256MiB/0.5 CPU；RabbitMQ 上限384MiB/1 CPU，限制 Erlang scheduler/async threads，使用轻量 TCP listener 探针；Agent 上限256MiB/0.5 CPU。Go 并行度2；镜像依赖构建内存上限512MiB。依赖先准备，再串行启动业务验证。

一键脚本使用独立 project/network，完成或失败均清理本项目的基础设施及匿名卷。Agent 清理须同时匹配本轮 private bootstrap hash 和资源标签；同名既有容器拒绝复用。基础镜像/构建缓存保留供后续复用。

## 验证

| 项目 | 结果/证据 |
|---|---|
| 首轮真实 Docker-native 联调 | 通过，94.267s；`/tmp/p8-pipeline.txt` |
| 一键脚本完整复验 | 通过，Go 联调100.403s，随后容器/network 清理完成；`/tmp/p8-one-command.txt` |
| Go 夹具编译/默认 suite | 通过；`/tmp/p8-unit.txt`，未显式开启的外部集成按原规则跳过 |
| race 构建/默认 suite | 通过；`/tmp/p8-race.txt`，该轮未开启 live 环境；不将其称为 live race 验收 |
| Python 新/改夹具 | ruff、format、mypy（explicit package bases）通过，两源文件 |
| 脚本与生成检查 | bash -n、gofmt、git diff --check 通过；无 IDL/schema 变更 |
| k3s 与 fixture 最终状态 | k3s inactive/dead，k3s shim为0；本阶段 Agent/数据库/Broker/network 已清理 |

## 复现入口与范围

仓库根运行 `harness/tools/agent_ownership_integration/run-docker.sh`，详见同目录 README。准备的 Agent 依赖镜像可由环境变量指定；首次准备基础依赖会联网安装，后续复用缓存。

Controller 和 Gateway 在宿主机以真实 HTTP 服务运行，Agent 在实际 Docker 容器中。宿主机 fixture 的受限 Dialer 仅将当前 Agent 的 Docker DNS 转换到原生容器 IP；生产同网络部署直接使用 Docker DNS。原生身份不是 mock。

Auth/Control 固定上下文和模型执行是明确 fixture；本阶段不声称已完成真实模型调用、外部 Plugin 授权或生产元数据配置部署。这些边界保持原有阶段证据，P8 重点补齐之前分离测试的 Docker 创建与 Agent 执行链路。
