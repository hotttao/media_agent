package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/protocol"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// ManagedExecutor adapts a configured internal capability implementation.
// It never carries an API Token or a user credential.
type ManagedExecutor struct {
	Client  *hclient.Client
	BaseURL string
}

func (e *ManagedExecutor) Execute(ctx context.Context, body []byte, idempotencyKey string) (map[string]any, error) {
	request, response := protocol.AcquireRequest(), protocol.AcquireResponse()
	defer protocol.ReleaseRequest(request)
	defer protocol.ReleaseResponse(response)
	request.SetRequestURI(strings.TrimRight(e.BaseURL, "/") + "/execute")
	request.Header.SetMethod("POST")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	headers := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, headers)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	request.SetBody(body)
	if err := e.Client.Do(ctx, request, response); err != nil {
		return nil, fmt.Errorf("managed executor unavailable")
	}
	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("managed executor status %d", response.StatusCode())
	}
	var output map[string]any
	if err := json.Unmarshal(response.Body(), &output); err != nil {
		return nil, fmt.Errorf("managed executor returned invalid JSON")
	}
	return output, nil
}
