package hello

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	hellomodel "media_agent/hertz_gen/model/media_example/hello"
)

// method1Cmd 是 HelloService.Method1 的三级子命令。
//
// proto 定义（idl/media_example/hello.proto）：
//
//	rpc Method1(HelloReq) returns(HelloResp) {
//	   option (api.get) = "/hello";
//	}
//
// 配置：base_domain 从 cfg 读（见 client.go 的 Init），不在命令上挂 --host flag
// （配置是 source of truth，flag 是临时 override——本命令遵循前者）。
func method1Cmd() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "method1",
		Short: "Method1(HelloReq) returns(HelloResp) — GET /hello",
		RunE: func(c *cobra.Command, args []string) error {
			cli := Client() // 共享 typed client（Init 在启动期已构造）
			resp, _, err := cli.Method1(context.Background(), &hellomodel.HelloReq{Name: name})
			if err != nil {
				return fmt.Errorf("call Method1: %w", err)
			}
			fmt.Println(resp.RespBody)
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "问候名称（空则回退 world）")
	return c
}

func init() {
	HelloCmd.AddCommand(method1Cmd())
}
