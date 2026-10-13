package workflow

import (
	"context"
	"encoding/json"
	"log/slog"
	catalog "media_agent/media_agent_gateway/biz/service/mcp_catalog"
	"time"
)

type MCPCatalogPublisher interface {
	Publish(context.Context, string, string, []byte) error
}

func RelayMCPCatalog(ctx context.Context, c *catalog.Service, publisher MCPCatalogPublisher) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			events, e := c.PendingEvents(ctx, 100)
			if e != nil {
				slog.ErrorContext(ctx, "MCP catalog outbox read failed")
				continue
			}
			for _, event := range events {
				body, _ := json.Marshal(map[string]any{"schema_version": 1, "event_type": "mcp.catalog.changed", "event_id": event.ID, "definition_id": event.DefinitionID, "definition_version": event.Version, "occurred_at": event.CreatedAt})
				publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				e = publisher.Publish(publishCtx, "mcp.catalog.changed", event.ID, body)
				cancel()
				if e == nil {
					e = c.CompleteEvent(ctx, event.ID)
				} else {
					_ = c.DeferEvent(ctx, event.ID)
				}
				if e != nil {
					slog.ErrorContext(ctx, "MCP catalog event delivery deferred", "event_id", event.ID)
				}
			}
		}
	}
}
