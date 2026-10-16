package workspace

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
)

var ErrModelSettings = errors.New("invalid model invocation settings")

func NormalizeModelSettings(agent, protocol, name, endpoint string) (string, string, string, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	name = strings.TrimSpace(name)
	endpoint = strings.TrimSpace(endpoint)
	if agent == "codex" {
		if protocol != "" || name != "" || endpoint != "" {
			return "", "", "", ErrModelSettings
		}
		return "", "", "", nil
	}
	if protocol == "" {
		if agent == "claude-code" {
			protocol = "anthropic"
		} else {
			protocol = "openai"
		}
	}
	if (agent == "claude-code" && protocol != "anthropic") || (agent == "goose" && protocol != "openai" && protocol != "anthropic") {
		return "", "", "", ErrModelSettings
	}
	if len(name) > 256 || strings.IndexFunc(name, unicode.IsControl) >= 0 || len(endpoint) > 2048 || strings.IndexFunc(endpoint, unicode.IsControl) >= 0 {
		return "", "", "", ErrModelSettings
	}
	if endpoint != "" {
		u, e := url.Parse(endpoint)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return "", "", "", ErrModelSettings
		}
		endpoint = strings.TrimRight(endpoint, "/")
	}
	return protocol, name, endpoint, nil
}
