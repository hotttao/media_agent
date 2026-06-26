package infra

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"media_agent/hertz_infra/config"
	"media_agent/media_cli/cmd"
)

// reloadConf config-reload 命令的 --conf flag。
var reloadConf string

// configReloadCmd 测试 YAML 文件变更触发热更新。
//
// 流程：
//  1. 用 hertz_infra/config 的 Loader 加载目标 YAML；
//  2. 修改 YAML 文件（改 app.version）；
//  3. 等待 watcher 触发 reload；
//  4. 检查 Loader.Current() 是否反映新值。
//
// --conf 行为：
//   - 为空：调用 config.Load()（.env + APP_ENV）解析路径；
//   - 非空：直接按该路径加载。
//
// 注意：该测试直接 import hertz_infra/config，
// 会引入 yaml.v3 + fsnotify + consul/api 依赖；因此仅在 e2e 环境用。
var configReloadCmd = &cobra.Command{
	Use:   "config-reload",
	Short: "测试 YAML 文件变更触发热更新",
	RunE: func(c *cobra.Command, args []string) error {
		// 1. 解析路径：--conf > CONFIG_FILE > APP_ENV 约定。
		path := reloadConf
		if path == "" {
			_ = godotenv.Load() // 与 config.Load 一致地灌 .env
			if p := os.Getenv("CONFIG_FILE"); p != "" {
				path = p
			} else {
				env := strings.TrimSpace(os.Getenv("APP_ENV"))
				if env == "" {
					env = "local"
				}
				path = fmt.Sprintf("conf/%s.yaml", env)
			}
		}

		// 2. 加载初始配置（NewLoaderFromPath 不读 .env，路径由上面解析）。
		loader, err := config.NewLoaderFromPath(path)
		if err != nil {
			return cmd.Fail("config-reload", "load "+path+": "+err.Error())
		}
		defer loader.Close()
		// 启动文件 watch（reload 依赖 fsnotify 推送，不主动 watch 等不到变更）。
		if err := loader.Watch(); err != nil {
			return cmd.Fail("config-reload", "start watch: "+err.Error())
		}
		origVersion := loader.Current().GetApp().GetVersion()
		if cmd.FlagVerbose {
			fmt.Printf("initial app.version=%s (conf=%s)\n", origVersion, path)
		}

		// 3. 备份原文件，修改 app.version。
		raw, err := os.ReadFile(path)
		if err != nil {
			return cmd.Fail("config-reload", err.Error())
		}
		backup := append([]byte(nil), raw...)
		newVersion := "e2e-reloaded-" + time.Now().Format("150405")
		// 简单替换 `version: <orig>` 行。
		modified := replaceYAMLField(string(raw), "version", origVersion, newVersion)
		if err := os.WriteFile(path, []byte(modified), 0644); err != nil {
			return cmd.Fail("config-reload", err.Error())
		}
		defer func() {
			abs, _ := filepath.Abs(path)
			_ = os.WriteFile(abs, backup, 0644)
		}()

		// 4. 等 watcher 触发（file.Watch 通常 1-3s 内）。
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if loader.Current().GetApp().GetVersion() == newVersion {
				return cmd.Pass("config-reload", fmt.Sprintf("version %s → %s", origVersion, newVersion))
			}
			time.Sleep(200 * time.Millisecond)
		}
		return cmd.Fail("config-reload", fmt.Sprintf("version not reloaded after 10s, still %s", loader.Current().GetApp().GetVersion()))
	},
}

// replaceYAMLField 把 YAML 里某 key 的旧值替换为新值（行级匹配，简单实现）。
func replaceYAMLField(content, key, oldVal, newVal string) string {
	target := key + ": " + oldVal
	replacement := key + ": " + newVal
	idx := indexOf(content, target)
	if idx < 0 {
		return content
	}
	return content[:idx] + replacement + content[idx+len(target):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func init() {
	configReloadCmd.Flags().StringVar(&reloadConf, "conf", "", "配置文件路径；为空则按 .env + APP_ENV 自动选择")
	infraCmd.AddCommand(configReloadCmd)
}
