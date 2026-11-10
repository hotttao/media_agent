package client

import (
	"context"
	"encoding/json"
	"errors"
	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/protocol"
	cfg "media_agent/hertz_gen/config"
	gateway "media_agent/hertz_gen/media_agent_gateway/chat_service"
	auth "media_agent/hertz_gen/media_auth/application_service"
	connections "media_agent/hertz_gen/media_control/model_connection_service"
	control "media_agent/hertz_gen/media_control/workspace_service"
	"media_agent/hertz_infra/clienthertz"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
	"mime"
	"net/http"
	"strings"
	"time"
)

type Clients struct {
	base        string
	stream      *http.Client
	auth        auth.Client
	control     control.Client
	connections connections.Client
	gateway     gateway.Client
}
type authKey struct{}

func withAuthorization(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, authKey{}, value)
}
func headers(next hclient.Endpoint) hclient.Endpoint {
	return func(ctx context.Context, req *protocol.Request, res *protocol.Response) error {
		value, _ := ctx.Value(authKey{}).(string)
		req.Header.Set("Authorization", value)
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := next(ctx, req, res); err != nil {
			return err
		}
		if res.StatusCode() < 200 || res.StatusCode() >= 300 {
			return &contract.Error{StatusCode: res.StatusCode()}
		}
		media, _, err := mime.ParseMediaType(string(res.Header.ContentType()))
		if err != nil || (media != "application/json" && !strings.HasSuffix(media, "+json")) || !json.Valid(res.Body()) || strings.TrimSpace(string(res.Body())) == "null" {
			return errors.New("invalid platform response")
		}
		return nil
	}
}
func New(base string) (*Clients, error) {
	suite := clienthertz.NewClientSuiteFromConfig(&cfg.Config{})
	// Default suite has no retry configuration. The host application's global observability is not reinitialized.
	h, e := suite.NewClient()
	if e != nil {
		return nil, errors.New("platform transport initialization failed")
	}
	h.Use(headers)
	decider := func(status int, _ *protocol.Response) bool { return status < 200 || status >= 300 }
	a, e := auth.NewApplicationServiceClient(base, auth.WithHertzClient(h), auth.WithResponseResultDecider(decider))
	if e != nil {
		return nil, e
	}
	c, e := control.NewWorkspaceServiceClient(base, control.WithHertzClient(h), control.WithResponseResultDecider(decider))
	if e != nil {
		return nil, e
	}
	m, e := connections.NewModelConnectionServiceClient(base, connections.WithHertzClient(h), connections.WithResponseResultDecider(decider))
	if e != nil {
		return nil, e
	}
	g, e := gateway.NewChatServiceClient(base, gateway.WithHertzClient(h), gateway.WithResponseResultDecider(decider))
	if e != nil {
		return nil, e
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &Clients{auth: a, control: c, connections: m, gateway: g, base: base, stream: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
