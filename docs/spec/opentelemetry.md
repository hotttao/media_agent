# Hertz + OTel(Trace+Metrics) + Zap + OTel Collector + Prometheus + Loki 完整生产方案
## 整体架构（方案A标准生产架构）
```
Hertz Service
├─ Zap 输出结构化日志(带trace_id) → Filebeat → Loki
├─ OTel Tracing(中间件自动生成span) → OTel Collector(OTLP) → Jaeger
└─ OTel Metrics(HTTP埋点) → 暴露/metrics → Prometheus拉取
统一可视化：Grafana 对接 Prometheus+Loki+Jaeger
中转核心：OTel Collector 统一接收全服务观测数据，做采样、过滤、分流
```
## 一、完整依赖导入
```bash
# Hertz 基础
go get github.com/cloudwego/hertz/pkg/app/server
# hertz otel 观测套件(trace+metrics中间件)
go get github.com/hertz-contrib/obs-opentelemetry
# hertz zap 日志适配器
go get github.com/hertz-contrib/logger/zap
# otel zap桥接 自动注入trace_id到日志
go get github.com/hertz-contrib/obs-opentelemetry/logging/zap
# zap本体
go get go.uber.org/zap
# 日志切割
go get github.com/natefinch/lumberjack
# otel sdk 基础
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/sdk
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
go get go.opentelemetry.io/otel/exporters/prometheus
go get github.com/prometheus/client_golang/prometheus
```

## 二、项目分层代码实现
### 1. config/otel.go OTel全局初始化（Trace+Metrics双底座）
```go
package config

import (
	"context"
	"log"
	"time"

	hertzotelprovider "github.com/hertz-contrib/obs-opentelemetry/provider"
	"go.opentelemetry.io/otel/sdk/metric"
)

// OtelConfig OTel全局配置
type OtelConfig struct {
	ServiceName    string
	OtlpEndpoint  string // otel collector grpc地址 127.0.0.1:4317
	MetricInterval time.Duration
}

var OtelProvider hertzotelprovider.OpenTelemetryProvider

// InitOTel 初始化Trace+Metrics
func InitOTel(c OtelConfig) func(context.Context) {
	opts := []hertzotelprovider.Option{
		hertzotelprovider.WithServiceName(c.ServiceName),
		hertzotelprovider.WithExportEndpoint(c.OtlpEndpoint),
		hertzotelprovider.WithInsecure(),
		// metrics配置
		hertzotelprovider.WithMetricReader(metric.NewManualReader()),
		hertzotelprovider.WithMetricInterval(c.MetricInterval),
	}
	OtelProvider = hertzotelprovider.NewOpenTelemetryProvider(opts...)
	return func(ctx context.Context) {
		if err := OtelProvider.Shutdown(ctx); err != nil {
			log.Printf("otel shutdown err: %v", err)
		}
	}
}
```

### 2. config/zap.go 日志初始化，绑定OTel自动注入trace
```go
package config

import (
	"github.com/cloudwego/hertz/pkg/common/hlog"
	hertzzap "github.com/hertz-contrib/logger/zap"
	"github.com/hertz-contrib/obs-opentelemetry/logging/zapotel"
	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

type ZapConfig struct {
	LogPath string // 日志文件路径
	Level   zapcore.Level
}

func InitZap(c ZapConfig) {
	// 日志切割
	lumberJackLogger := &lumberjack.Logger{
		Filename:   c.LogPath,
		MaxSize:    100, // MB
		MaxBackups: 10,
		MaxAge:     7,
		Compress:   true,
	}
	// 多输出：文件+控制台
	writeSyncer := zapcore.NewMultiWriteSyncer(
		zapcore.AddSync(lumberJackLogger),
		zapcore.AddSync(os.Stdout),
	)
	// 生产JSON格式
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, writeSyncer, c.Level)
	rawZap := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	// 替换hertz默认日志
	hZap := hertzzap.NewLogger(hertzzap.WithZapLogger(rawZap))
	hlog.SetLogger(hZap)

	// 包装otel桥接：自动从ctx读取trace_id span_id
	traceLogger := zapotel.NewLogger(
		zapotel.WithLogger(hZap),
		zapotel.WithTraceIDField(true),
		zapotel.WithSpanIDField(true),
		zapotel.WithRecordStackTraceInSpan(true),
	)
	hlog.SetLogger(traceLogger)
}
```

### 3. main.go Hertz服务主入口，注册otel中间件
```go
package main

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	hertztracing "github.com/hertz-contrib/obs-opentelemetry/tracing"
	hertzmetrics "github.com/hertz-contrib/obs-opentelemetry/metrics"
	"your-project/config"
	"go.uber.org/zap/zapcore"
	"time"
)

func main() {
	// 1.初始化Zap日志
	config.InitZap(config.ZapConfig{
		LogPath: "./logs/app.log",
		Level:   zapcore.InfoLevel,
	})

	// 2.初始化OTel Trace+Metrics
	shutdownOtel := config.InitOTel(config.OtelConfig{
		ServiceName:    "hertz-demo-service",
		OtlpEndpoint:   "127.0.0.1:4317", // otel collector地址
		MetricInterval: 15 * time.Second,
	})
	defer shutdownOtel(context.Background())

	// 3.Hertz服务初始化
	h := server.Default(server.WithHostPorts("0.0.0.0:8080"))

	// 注册链路中间件
	tracerCfg, _ := hertztracing.NewServerTracer()
	h.Use(hertztracing.ServerMiddleware(tracerCfg))

	// 注册metrics指标中间件，自动暴露 /metrics
	metricCfg := hertzmetrics.NewConfig()
	h.Use(hertzmetrics.ServerMiddleware(metricCfg))

	// 测试路由
	h.GET("/test", func(ctx context.Context, c *app.RequestContext) {
		// 必须传入ctx，日志自动携带trace_id
		hlog.CtxInfof(ctx, "receive request path=%s", c.Request.Path())
		// 模拟错误日志
		hlog.CtxErrorf(ctx, "demo error log, uid=%d", 10001)
		c.JSON(200, map[string]string{"msg": "success"})
	})

	h.Spin()
}
```

## 三、外部基础服务部署（Docker Compose 一键启动全套观测组件）
`docker-compose.yml`
```yaml
version: "3"
services:
  # 1. OTel Collector 核心中转
  otel-collector:
    image: otel/opentelemetry-collector-contrib:latest
    command: ["--config=/etc/otel-collector-config.yaml"]
    volumes:
      - ./otel-collector.yaml:/etc/otel-collector-config.yaml
    ports:
      - "4317:4317" # otlp grpc trace接收
    depends_on:
      - jaeger
      - loki

  # 2. Jaeger 链路存储UI
  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "16686:16686" # web ui
      - "14250:14250"

  # 3. Loki 日志存储
  loki:
    image: grafana/loki:latest
    ports:
      - "3100:3100"
    command: -config.file=/etc/loki/local-config.yaml

  # 4. Prometheus 指标存储
  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"

  # 5. Grafana 统一可视化面板
  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    depends_on:
      - prometheus
      - loki
      - jaeger

  # 6. Filebeat 采集zap本地日志推送Loki
  filebeat:
    image: docker.elastic.co/beats/filebeat:8.11.0
    volumes:
      - ./filebeat.yml:/usr/share/filebeat/filebeat.yml
      - ./logs:/usr/share/filebeat/logs
```

### 配套配置文件
1. `otel-collector.yaml` 采集分流配置
```yaml
receivers:
  otlp:
    protocols:
      grpc:
exporters:
  trace/jaeger:
    endpoint: jaeger:14250
    tls:
      insecure: true
  logging:
processors:
  batch:
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [trace/jaeger, logging]
```

2. `prometheus.yml` 抓取Hertz服务metrics
```yaml
global:
  scrape_interval: 15s
scrape_configs:
  - job_name: "hertz-service"
    static_configs:
      - targets: ["host.docker.internal:8080"]
```

3. `filebeat.yml` 采集zap日志推送Loki
```yaml
filebeat.inputs:
- type: filestream
  paths:
    - /usr/share/filebeat/logs/*.log
output.loki:
  hosts: ["http://loki:3100"]
```

## 四、全链路数据流转说明
1. **Trace链路**
Hertz otel中间件生成span → grpc推送到OTel Collector → 批量转发Jaeger，访问`127.0.0.1:16686`通过trace_id查询整条调用链
2. **Metrics指标**
Hertz自动暴露`/metrics`接口 → Prometheus定时拉取存储 → Grafana配置大盘查看QPS、P99耗时、错误率
3. **Zap日志**
服务本地输出JSON日志(自动携带trace_id) → Filebeat采集文件 → Loki存储 → Grafana输入trace_id检索对应全量日志

## 五、生产关键规范&踩坑点
1. 所有业务日志必须使用`hlog.CtxXX(ctx, ...)`，不传ctx不会注入trace_id；
2. 程序退出必须执行`defer shutdownOtel`，否则链路数据丢失；
3. OTel Collector是生产必需，集群多服务统一收敛观测数据，避免各服务直连Jaeger/Loki造成网络压力；
4. Grafana添加3个数据源：Prometheus、Loki、Jaeger，实现指标、日志、链路一站式查看；
5. 高流量接口配置OTel采样策略，降低存储成本，异常请求强制全采样。

## 六、组件访问地址
- Hertz服务：http://127.0.0.1:8080
- Prometheus：http://127.0.0.1:9090
- Jaeger链路UI：http://127.0.0.1:16686
- Grafana可视化：http://127.0.0.1:3000
- Loki日志接口：http://127.0.0.1:3100
- OTel Collector OTLP接收端口：127.0.0.1:4317

## 需求:
你按照 @docs/spec/opentelemetry.md 完善 hertz_infra 里的代码，需要启动的服务，放在 @docker-compose 下通过 docker 启动。各个服务的数据目录统一放在 /data
  目录下，每一个服务一个目录，基础服务的数据和日志都放在服务自己的目录下。metrics/log/trace 默认实现继承到 media_example 和 media-cli 中。后续作为 e2e 测试的基础环境。media_logs
  用于存放各个各个为服务的日至，每一个服务一个目录。请规划完成代码。不要直接参考 @docs/spec/opentelemetry.md 代码实现，代码实现需要参考  harness/contributing/ 下 config.md server.md client.md 的规范开发。先把  plan 更新到 docs/plan 目录下，然后在规划执行

下面是官方文档:
1. https://www.cloudwego.io/zh/docs/hertz/tutorials/third-party/open-telemetry/#metric
2. https://www.cloudwego.io/zh/docs/hertz/tutorials/third-party/prometheus/
3. https://www.cloudwego.io/zh/docs/hertz/tutorials/third-party/log/zap/

你可以通过 https://r.jina.ai/${url} 获取文档对应的 markdown 版本，你需要结合文档提供的最佳实践。