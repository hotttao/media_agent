package application_caller

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	pb "media_agent/hertz_gen/model/media_agent_gateway/application_caller"
	"media_agent/hertz_infra/authn/applicationcaller"
)

var signer *applicationcaller.Signer

func Configure(s *applicationcaller.Signer) { signer = s }
func GetApplicationCallerKeys(ctx context.Context, c *app.RequestContext) {
	if signer == nil {
		c.JSON(503, map[string]string{"error": "caller signing unavailable"})
		return
	}
	r := &pb.CallerJWKS{}
	for _, k := range signer.PublicJWKS().Keys {
		r.Keys = append(r.Keys, &pb.CallerJWK{Kty: k.Kty, Use: k.Use, Alg: k.Alg, Kid: k.Kid, N: k.N, E: k.E})
	}
	c.Header("Cache-Control", "public, max-age=60")
	c.JSON(200, r)
}
