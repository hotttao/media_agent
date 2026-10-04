package media_agent_gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	hconfig "github.com/cloudwego/hertz/pkg/common/config"
	"github.com/spf13/cobra"
	pb "media_agent/hertz_gen/model/media_agent_gateway/agent_proxy"
)

func agentRequestCmd() *cobra.Command {
	var service, method, uri, contentType, bodyJSON, bodyFile string
	command := &cobra.Command{Use: "request", Short: "使用 Agent API Token 通过网关调用业务服务", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if agentProxyClient == nil {
			return fmt.Errorf("Agent proxy client is not configured")
		}
		var body []byte
		if bodyJSON != "" {
			if !json.Valid([]byte(bodyJSON)) {
				return fmt.Errorf("--body-json must be valid JSON")
			}
			body = []byte(bodyJSON)
			if contentType == "" {
				contentType = "application/json"
			}
		}
		if bodyFile != "" {
			f, err := os.Open(bodyFile)
			if err != nil {
				return fmt.Errorf("cannot open request body file")
			}
			defer f.Close()
			body, err = io.ReadAll(io.LimitReader(f, (8<<20)+1))
			if err != nil {
				return fmt.Errorf("cannot read request body file")
			}
			if contentType == "" {
				return fmt.Errorf("--content-type is required with --body-file")
			}
		}
		if len(body) > 8<<20 {
			return fmt.Errorf("request body exceeds 8 MiB")
		}
		response, _, err := agentProxyClient.ProxyAgentRequest(c.Context(), &pb.ProxyAgentRequestBody{Service: service, Method: method, Uri: uri, ContentType: contentType, Body: body}, hconfig.WithRequestTimeout(25*time.Second))
		if err != nil {
			return fmt.Errorf("Agent proxy request failed; no automatic retry was attempted")
		}
		if response == nil || response.StatusCode < 100 || response.StatusCode > 599 {
			return fmt.Errorf("Agent proxy response incomplete")
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("Agent upstream status=%d trace_id=%s", response.StatusCode, response.TraceId)
		}
		var output any = response.Body
		if json.Valid(response.Body) {
			output = json.RawMessage(response.Body)
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"status_code": response.StatusCode, "content_type": response.ContentType, "body": output, "trace_id": response.TraceId})
	}}
	command.Flags().StringVar(&service, "service", "", "workflow 或 materials")
	command.Flags().StringVar(&method, "method", "GET", "HTTP 方法")
	command.Flags().StringVar(&uri, "uri", "", "业务 API 路径和查询参数，不接受外部 URL")
	command.Flags().StringVar(&contentType, "content-type", "", "原始请求 Content-Type")
	command.Flags().StringVar(&bodyJSON, "body-json", "", "JSON 请求正文")
	command.Flags().StringVar(&bodyFile, "body-file", "", "二进制或 multipart 请求正文文件")
	_ = command.MarkFlagRequired("service")
	_ = command.MarkFlagRequired("uri")
	command.MarkFlagsMutuallyExclusive("body-json", "body-file")
	return command
}
