package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"

	configpb "media_agent/hertz_gen/config"
)

// Config 是 configpb.Config 的别名，方便调用方 import：
//
//	import "media_agent/hertz_infra/config"
//	var cfg *config.Config
type Config = configpb.Config

// godotenvLoad 封装 godotenv.Load，供 NewLoader 复用。
func godotenvLoad() error {
	return godotenv.Load()
}

// resolveConfigPath 决定加载哪个 YAML 文件。
//
//	CONFIG_FILE 优先；否则按 APP_ENV（默认 local）选 conf/<env>.yaml。
func resolveConfigPath() string {
	if p := os.Getenv("CONFIG_FILE"); p != "" {
		return p
	}
	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	if env == "" {
		env = "local"
	}
	return fmt.Sprintf("conf/%s.yaml", env)
}
