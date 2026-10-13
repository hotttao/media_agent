package middleware

import (
	"context"
	"crypto/subtle"
	"github.com/cloudwego/hertz/pkg/app"
)

var metadataGatewayCredential string

func ConfigureMetadataGateway(credential string) { metadataGatewayCredential = credential }
func MetadataGateway() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		provided := string(c.GetHeader("X-Metadata-Gateway-Credential"))
		if metadataGatewayCredential == "" || provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(metadataGatewayCredential)) != 1 {
			c.JSON(403, map[string]string{"error": "metadata_gateway_identity_required"})
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}
