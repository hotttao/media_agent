package media_agent_gateway

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/protocol"
	pb "media_agent/hertz_gen/model/media_agent_gateway/agent_proxy"
)

type proxyClientStub struct {
	status  int
	calls   int
	request *pb.ProxyAgentRequestBody
}

func (s *proxyClientStub) ProxyAgentRequest(_ context.Context, r *pb.ProxyAgentRequestBody, _ ...config.RequestOption) (*pb.ProxyAgentResponse, *protocol.Response, error) {
	s.calls++
	s.request = r
	return &pb.ProxyAgentResponse{StatusCode: int32(s.status), ContentType: "application/json", Body: []byte(`{"ok":true}`), TraceId: "trace"}, nil, nil
}
func (s *proxyClientStub) GetAgentJWKS(context.Context, *pb.GetAgentJWKSRequest, ...config.RequestOption) (*pb.AgentJWKS, *protocol.Response, error) {
	return nil, nil, nil
}

func TestAgentRequestCLIRejectsNonSuccessAndDoesNotRetry(t *testing.T) {
	previous := agentProxyClient
	t.Cleanup(func() { agentProxyClient = previous })
	for _, status := range []int{200, 201, 202, 307, 401, 409, 503} {
		stub := &proxyClientStub{status: status}
		agentProxyClient = stub
		command := agentRequestCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"--service", "materials", "--method", "POST", "--uri", "/api/v1/materials", "--body-json", `{"name":"test"}`})
		err := command.Execute()
		if (err == nil) != (status >= 200 && status < 300) || stub.calls != 1 {
			t.Fatalf("status=%d unexpected success or retry", status)
		}
		if stub.request.ContentType != "application/json" || string(stub.request.Body) != `{"name":"test"}` {
			t.Fatal("CLI request body mapping lost")
		}
		if status >= 200 && status < 300 && !strings.Contains(output.String(), `"ok":true`) {
			t.Fatal("CLI did not decode JSON business response")
		}
	}
	stub := &proxyClientStub{status: 200}
	agentProxyClient = stub
	command := agentRequestCmd()
	command.SetArgs([]string{"--service", "materials", "--uri", "/api/v1/materials", "--body-json", "{"})
	if err := command.Execute(); err == nil || stub.calls != 0 {
		t.Fatal("malformed JSON reached the proxy")
	}
}
