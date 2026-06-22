// serve.go：启动一个用于 e2e 测试的 hertz server。
//
// 该 server 通过 hertz_infra/serverhertz 装配全部治理能力，并额外暴露测试端点：
//
//	GET  /_test/echo           ─ 原样回显 query
//	GET  /_test/panic          ─ 触发 panic（验证 recovery）
//	GET  /_test/slow?ms=N      ─ 延迟 N 毫秒（验证 timeout）
//	GET  /_test/error?code=N   ─ 返回 N 状态码（验证 retry / circuit breaker）
//
// 配置加载逻辑：
//   - --conf 未指定 → chdir 到 media_example/ 后调 config.Load()
//     （加载 .env，按 APP_ENV 选 conf/<env>.yaml）；
//   - --conf 指定 → 直接 LoadFromPath，跳过 .env / APP_ENV。
package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/spf13/cobra"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
)

var serveConf string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "启动 e2e 测试 server（带治理能力 + 测试端点）",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := loadLoaderOrDie(serveConf)
		suite := serverhertz.NewServerSuite(loader)
		h := serverhertz.NewServer(suite)
		mountTestEndpoints(h)
		log.Printf("serve: listening on %s (conf=%s)", loader.Current().GetServer().GetAddress(), confDesc(serveConf))
		if err := suite.RegisterService(); err != nil {
			log.Printf("serve: register service: %v", err)
		}
		defer func() { _ = suite.DeregisterService() }()
		h.Spin()
		return nil
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveConf, "conf", "", "配置文件路径；为空则按 .env + APP_ENV 自动选择（chdir 到 media_example/）")
	RegisterCmd(serveCmd)
}

// loadLoaderOrDie 加载配置返回 *config.Loader，失败 fatal。
//
// path 为空时 chdir 到 media_example/ 后走 config.NewLoader()（.env + APP_ENV）；
// path 非空时走 config.NewLoaderFromPath(path)，跳过 .env / APP_ENV。
func loadLoaderOrDie(path string) *config.Loader {
	var (
		loader *config.Loader
		err    error
	)
	if path == "" {
		// config.NewLoader() 依赖相对路径 .env 和 conf/<env>.yaml，
		// 必须在 media_example/ 目录下执行。
		if err := chdirToMediaExample(); err != nil {
			log.Fatalf("load config: %v", err)
		}
		loader, err = config.NewLoader()
	} else {
		loader, err = config.NewLoaderFromPath(path)
	}
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	// serve 命令是一次性 e2e 测试，loader 保持运行直到进程退出，不需要 Close。
	return loader
}

// chdirToMediaExample 把工作目录切到 media_example/，
// 让 config.Load() 的相对路径 (.env, conf/<env>.yaml) 能解析到。
//
// 从 media-cli/ 启动时，相对路径 ../media_example。
func chdirToMediaExample() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	// 已经在 media_example/ 里（通过 go run 在 media_example 目录下启动 media-cli 源码）。
	if filepath.Base(wd) == "media_example" {
		return nil
	}
	target := filepath.Join(wd, "..", "media_example")
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("locate media_example at %s: %w", target, err)
	}
	return os.Chdir(target)
}

// confDesc 给日志用，描述实际加载的路径来源。
func confDesc(path string) string {
	if path != "" {
		return path
	}
	return "<.env + APP_ENV>"
}

// mountTestEndpoints 挂载 e2e 测试专用端点。
func mountTestEndpoints(h *server.Hertz) {
	h.GET("/_test/echo", func(ctx context.Context, c *app.RequestContext) {
		c.JSON(consts.StatusOK, map[string]string{
			"msg":  "ok",
			"name": string(c.Query("name")),
		})
	})

	h.GET("/_test/panic", func(ctx context.Context, c *app.RequestContext) {
		panic("intentional panic for e2e recovery test")
	})

	h.GET("/_test/slow", func(ctx context.Context, c *app.RequestContext) {
		ms := c.Query("ms")
		n, _ := strconv.Atoi(string(ms))
		if n <= 0 {
			n = 1000
		}
		time.Sleep(time.Duration(n) * time.Millisecond)
		c.JSON(consts.StatusOK, map[string]any{"slept_ms": n})
	})

	h.GET("/_test/error", func(ctx context.Context, c *app.RequestContext) {
		code := c.Query("code")
		n, _ := strconv.Atoi(string(code))
		if n == 0 {
			n = 500
		}
		c.JSON(n, map[string]string{"err": fmt.Sprintf("forced %d", n)})
	})
}
