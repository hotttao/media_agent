package client

import "media_agent/hertz_infra/mcpupstream"

type MCPBackend = mcpupstream.MCPBackend
type MCPUpstream = mcpupstream.MCPUpstream
type MCPPeer = mcpupstream.MCPPeer

var ErrMCPReauth = mcpupstream.ErrMCPReauth
var ErrMCPUpstream = mcpupstream.ErrMCPUpstream
