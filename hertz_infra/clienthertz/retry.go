// retry.go：客户端重试。
//
// 走 hertz client.WithRetryConfig + retry.WithMaxAttemptTimes 等。
package clienthertz

import (
	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/app/client/retry"

	configpb "media_agent/hertz_gen/config"
)

func buildRetryOptions(r *configpb.RetryConfig) []config.ClientOption {
	if r == nil || !r.GetEnabled() {
		return nil
	}
	rOpts := []retry.Option{}
	if v := r.GetMaxAttemptTimes(); v > 0 {
		rOpts = append(rOpts, retry.WithMaxAttemptTimes(uint(v)))
	}
	if v := r.GetInitialDelayMs(); v > 0 {
		rOpts = append(rOpts, retry.WithInitDelay(msToDuration(v)))
	}
	if v := r.GetMaxDelayMs(); v > 0 {
		rOpts = append(rOpts, retry.WithMaxDelay(msToDuration(v)))
	}
	if v := r.GetDelayMultiplier(); v > 0 {
		rOpts = append(rOpts, retry.WithMaxJitter(msToDuration(int32(v*1000))))
	}
	return []config.ClientOption{hclient.WithRetryConfig(rOpts...)}
}
