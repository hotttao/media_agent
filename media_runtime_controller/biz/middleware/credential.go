package middleware

import (
	"context"
	"crypto/subtle"
	"github.com/cloudwego/hertz/pkg/app"
)

var credential string

func Configure(value string) { credential = value }
func Creation() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		provided := string(c.GetHeader("X-Runtime-Creation-Credential"))
		if credential == "" || len(provided) != len(credential) || subtle.ConstantTimeCompare([]byte(provided), []byte(credential)) != 1 {
			c.AbortWithStatus(401)
			return
		}
		c.Next(ctx)
	}
}
