// infra_consul.go：consul 注册 / 发现 e2e。
//
// 两个子命令：
//
//	infra consul register  ─ 查询 consul catalog，验证 media-example 已注册
//	infra consul discover  ─ 通过 clienthertz 装配 discovery client 并调用
package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/spf13/cobra"

	configpb "media_agent/hertz_gen/config"
)

var consulAddr string

var infraConsulCmd = &cobra.Command{
	Use:   "consul",
	Short: "Consul 注册 / 发现 e2e 测试",
}

var infraConsulRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "验证服务已注册到 Consul",
	RunE: func(cmd *cobra.Command, args []string) error {
		cc, err := consulapi.NewClient(&consulapi.Config{Address: consulAddr})
		if err != nil {
			return fail("consul register", err.Error())
		}
		svcs, _, err := cc.Catalog().Services(nil)
		if err != nil {
			return fail("consul register", err.Error())
		}
		if _, ok := svcs["media-example"]; !ok {
			keys := make([]string, 0, len(svcs))
			for k := range svcs {
				keys = append(keys, k)
			}
			return fail("consul register", fmt.Sprintf("media-example not in catalog: %v", keys))
		}
		instances, _, err := cc.Health().Service("media-example", "", false, nil)
		if err != nil {
			return fail("consul register", err.Error())
		}
		return pass("consul register", fmt.Sprintf("%d instance(s) registered", len(instances)))
	},
}

var infraConsulDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "通过 Consul 发现并调用 media-example",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 直接用 consul HTTP API 查询 service，再用普通 HTTP client 调用，
		// 验证 discovery 链路通；不依赖 clienthertz（clienthertz discovery 是 TODO）。
		cc, err := consulapi.NewClient(&consulapi.Config{Address: consulAddr})
		if err != nil {
			return fail("consul discover", err.Error())
		}
		instances, _, err := cc.Health().Service("media-example", "", true, nil)
		if err != nil {
			return fail("consul discover", err.Error())
		}
		if len(instances) == 0 {
			return fail("consul discover", "no healthy instance")
		}
		inst := instances[0]
		target := fmt.Sprintf("http://%s:%d/health", inst.Service.Address, inst.Service.Port)
		resp, err := http.Get(target)
		if err != nil {
			return fail("consul discover", err.Error())
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fail("consul discover", fmt.Sprintf("status=%d", resp.StatusCode))
		}
		return pass("consul discover", target)
	},
}

func init() {
	infraConsulCmd.PersistentFlags().StringVar(&consulAddr, "consul", "127.0.0.1:8500", "Consul agent 地址")
	infraConsulCmd.AddCommand(infraConsulRegisterCmd)
	infraConsulCmd.AddCommand(infraConsulDiscoverCmd)
	infraCmd.AddCommand(infraConsulCmd)

	// 避免 unused 警告。
	_ = (*configpb.Config)(nil)
	_ = json.Marshal
}
