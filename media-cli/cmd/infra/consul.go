package infra

import (
	"fmt"
	"net/http"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// consulAddr consul agent 地址（--consul flag）。
var consulAddr string

// infraConsulCmd 是 consul 子命令的父命令；其下挂 register / discover。
var infraConsulCmd = &cobra.Command{
	Use:   "consul",
	Short: "Consul 注册 / 发现 e2e 测试",
}

// infraConsulRegisterCmd 验证服务已注册到 Consul。
var infraConsulRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "验证服务已注册到 Consul",
	RunE: func(c *cobra.Command, args []string) error {
		cc, err := consulapi.NewClient(&consulapi.Config{Address: consulAddr})
		if err != nil {
			return cmd.Fail("consul register", err.Error())
		}
		svcs, _, err := cc.Catalog().Services(nil)
		if err != nil {
			return cmd.Fail("consul register", err.Error())
		}
		if _, ok := svcs["media-example"]; !ok {
			keys := make([]string, 0, len(svcs))
			for k := range svcs {
				keys = append(keys, k)
			}
			return cmd.Fail("consul register", fmt.Sprintf("media-example not in catalog: %v", keys))
		}
		instances, _, err := cc.Health().Service("media-example", "", false, nil)
		if err != nil {
			return cmd.Fail("consul register", err.Error())
		}
		return cmd.Pass("consul register", fmt.Sprintf("%d instance(s) registered", len(instances)))
	},
}

// infraConsulDiscoverCmd 通过 Consul 发现并调用 media-example。
var infraConsulDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "通过 Consul 发现并调用 media-example",
	RunE: func(c *cobra.Command, args []string) error {
		// 直接用 consul HTTP API 查询 service，再用普通 HTTP client 调用，
		// 验证 discovery 链路通；不依赖 clienthertz（clienthertz discovery 是 TODO）。
		cc, err := consulapi.NewClient(&consulapi.Config{Address: consulAddr})
		if err != nil {
			return cmd.Fail("consul discover", err.Error())
		}
		instances, _, err := cc.Health().Service("media-example", "", true, nil)
		if err != nil {
			return cmd.Fail("consul discover", err.Error())
		}
		if len(instances) == 0 {
			return cmd.Fail("consul discover", "no healthy instance")
		}
		inst := instances[0]
		target := fmt.Sprintf("http://%s:%d/health", inst.Service.Address, inst.Service.Port)
		resp, err := http.Get(target)
		if err != nil {
			return cmd.Fail("consul discover", err.Error())
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return cmd.Fail("consul discover", fmt.Sprintf("status=%d", resp.StatusCode))
		}
		return cmd.Pass("consul discover", target)
	},
}

func init() {
	infraConsulCmd.PersistentFlags().StringVar(&consulAddr, "consul", "127.0.0.1:8500", "Consul agent 地址")
	infraConsulCmd.AddCommand(infraConsulRegisterCmd)
	infraConsulCmd.AddCommand(infraConsulDiscoverCmd)
	infraCmd.AddCommand(infraConsulCmd)
}
