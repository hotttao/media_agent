// timeout.go：客户端 dial / read / write / 整体超时。
package clienthertz

import (
	"time"

	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/common/config"

	configpb "media_agent/hertz_gen/config"
)

func buildTimeoutOptions(t *configpb.TimeoutConfig) []config.ClientOption {
	if t == nil || !t.GetEnabled() {
		return nil
	}
	opts := []config.ClientOption{}
	if v := t.GetConnectTimeoutMs(); v > 0 {
		opts = append(opts, hclient.WithDialTimeout(msToDuration(v)))
	}
	if v := t.GetReadTimeoutMs(); v > 0 {
		opts = append(opts, hclient.WithClientReadTimeout(msToDuration(v)))
	}
	if v := t.GetWriteTimeoutMs(); v > 0 {
		opts = append(opts, hclient.WithWriteTimeout(msToDuration(v)))
	}
	return opts
}

func msToDuration(ms int32) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
