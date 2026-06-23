# jaeger

Jaeger all-in-one，无独立配置文件——行为通过环境变量控制（见 docker-compose.yml
的 `jaeger.service.environment`）：

- `COLLECTOR_OTLP_ENABLED=true`：开启 OTLP 接收（collector 经 `jaeger:4317` 推送）。
- `SPAN_STORAGE_TYPE=badger` + `BADGER_DIRECTORY_VALUE/KEY`：本地 badger 存储，数据落 `/data/jaeger/badger`。

数据目录：`/data/jaeger/badger`（compose 挂载）。
