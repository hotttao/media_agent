package platformsdk

import "media_agent/packages/platform-go-sdk/biz/shared/caller"

type CallerConfig = caller.Config
type CallerIdentity = caller.Identity
type CallerBinding = caller.Binding
type CallerVerifier = caller.Verifier

// NewCallerVerifier pins trusted deployment JWKS, issuer and application audience.
func NewCallerVerifier(c CallerConfig) (*CallerVerifier, error) { return caller.New(c) }
