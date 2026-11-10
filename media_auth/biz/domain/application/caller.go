package application

import (
	"context"
	"net/url"
	"strings"
)

type MCPTarget struct{ ApplicationID, DefinitionID, Origin string }
type CallerRepository interface {
	PutTarget(context.Context, MCPTarget) error
	DeleteTarget(context.Context, string, string) error
	Target(context.Context, string) (MCPTarget, error)
	CallerProfile(context.Context, string, string) (Profile, error)
}

func CanonicalOrigin(raw string) (string, bool) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", false
	}
	host := strings.ToLower(u.Host)
	if strings.HasSuffix(host, ":443") {
		host = strings.TrimSuffix(host, ":443")
	}
	return "https://" + host, true
}
