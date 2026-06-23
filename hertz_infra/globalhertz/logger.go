// logger.go：结构化应用日志全局初始化（zap + lumberjack + zapotel 桥接）。
//
// 对齐官方文档（cloudwego.io Hertz zap）：
//
//	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
//	l := hertzzap.NewLogger(hertzzap.WithCores(hertzzap.CoreConfig{Enc: enc, Ws: ws, Lvl: lvl}))
//
// 再用 obs-opentelemetry/logging/zap 包装，自动从 context 注入 trace_id / span_id：
//
//	traceLogger := zapotel.NewLogger(zapotel.WithLogger(hZap))
//	hlog.SetLogger(traceLogger)
package globalhertz

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	hertzzap "github.com/hertz-contrib/logger/zap"
	zapotel "github.com/hertz-contrib/obs-opentelemetry/logging/zap"
	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	configpb "media_agent/hertz_gen/config"
)

var loggerOnce sync.Once

func initLogger(app *configpb.AppConfig, c *configpb.LogConfig) error {
	if c == nil || !c.GetEnabled() {
		return nil
	}

	var (
		level       = parseLevel(c.GetLevel(), zapcore.InfoLevel)
		writeSyncer zapcore.WriteSyncer
	)

	// 文件输出：lumberjack 切割；目录缺失则创建。
	fileWS, err := fileWriteSyncer(c)
	if err != nil {
		return fmt.Errorf("globalhertz: logger file: %w", err)
	}
	if fileWS != nil {
		if c.GetConsole() {
			writeSyncer = zapcore.NewMultiWriteSyncer(fileWS, zapcore.AddSync(os.Stdout))
		} else {
			writeSyncer = fileWS
		}
	} else {
		// 无文件路径时仅控制台。
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	atomicLevel := zap.NewAtomicLevelAt(level)
	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())

	hZap := hertzzap.NewLogger(
		hertzzap.WithCores(hertzzap.CoreConfig{Enc: enc, Ws: writeSyncer, Lvl: atomicLevel}),
		hertzzap.WithZapOptions(
			zap.AddCaller(),
			zap.AddStacktrace(zapcore.ErrorLevel),
		),
	)

	// zapotel 桥接：包装 hertzzap.Logger，自动从 ctx 注入 trace_id / span_id。
	traceLogger := zapotel.NewLogger(zapotel.WithLogger(hZap))

	loggerOnce.Do(func() {
		hlog.SetLogger(traceLogger)
	})
	return nil
}

// fileWriteSyncer 构造 lumberjack 文件 WriteSyncer；filename 为空返回 nil（仅控制台）。
func fileWriteSyncer(c *configpb.LogConfig) (zapcore.WriteSyncer, error) {
	dir := c.GetDir()
	filename := c.GetFilename()
	if filename == "" {
		filename = "app.log"
	}
	if dir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	lj := &lumberjack.Logger{
		Filename:   filepath.Join(dir, filename),
		MaxSize:    intOrDefault(c.GetMaxSizeMb(), 100),
		MaxBackups: intOrDefault(c.GetMaxBackups(), 10),
		MaxAge:     intOrDefault(c.GetMaxAgeDays(), 7),
		Compress:   c.GetCompress(),
	}
	return zapcore.AddSync(lj), nil
}

// parseLevel 把字符串级别映射为 zapcore.Level，未知/空走 fallback。
func parseLevel(s string, fallback zapcore.Level) zapcore.Level {
	switch s {
	case "debug", "DEBUG":
		return zapcore.DebugLevel
	case "info", "INFO":
		return zapcore.InfoLevel
	case "warn", "WARN", "warning":
		return zapcore.WarnLevel
	case "error", "ERROR":
		return zapcore.ErrorLevel
	case "fatal", "FATAL":
		return zapcore.FatalLevel
	default:
		return fallback
	}
}

// intOrDefault 返回 >0 的值，否则 fallback。
func intOrDefault(v, fallback int32) int {
	if v > 0 {
		return int(v)
	}
	return int(fallback)
}
