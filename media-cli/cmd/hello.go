// Package cmd 存放 media-cli 各子命令。
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	helloclient "media_agent/hertz_gen/media_example"
	hellomodel "media_agent/hertz_gen/model/media_example/hello"
)

// helloCmd 调用 media_example 的 HelloService.Method1（GET /hello）。
func newHelloCmd() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "hello",
		Short: "调用 HelloService.Method1（GET /hello）",
		RunE: func(cmd *cobra.Command, args []string) error {
			host, _ := cmd.Flags().GetString("host")
			client, err := helloclient.NewHelloServiceClient(host)
			if err != nil {
				return fmt.Errorf("create hello client: %w", err)
			}
			resp, _, err := client.Method1(context.Background(), &hellomodel.HelloReq{Name: name})
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
	RegisterCmd(newHelloCmd())
}
