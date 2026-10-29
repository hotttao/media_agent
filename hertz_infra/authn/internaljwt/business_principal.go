package internaljwt

import "context"

// BusinessPrincipal is the verified scope consumed by resource owners. It does
// not imply an Agent actor, a Worker role, or any business permission.
type BusinessPrincipal struct {
	UserID, ActorID, OrganizationID, WorkspaceID, CredentialID string
}
type businessPrincipalKey struct{}

// ContextWithBusinessPrincipal is used only after the receiving service has
// verified the request's dedicated authentication/delegation contract.
func ContextWithBusinessPrincipal(ctx context.Context, p BusinessPrincipal) context.Context {
	return context.WithValue(ctx, businessPrincipalKey{}, p)
}
func BusinessPrincipalFromContext(ctx context.Context) (BusinessPrincipal, bool) {
	if p, ok := ctx.Value(businessPrincipalKey{}).(BusinessPrincipal); ok {
		return p, true
	}
	p, ok := AgentPrincipalFromContext(ctx)
	if !ok {
		return BusinessPrincipal{}, false
	}
	return BusinessPrincipal{UserID: p.UserID, ActorID: p.ActorID, OrganizationID: p.OrganizationID, WorkspaceID: p.WorkspaceID, CredentialID: p.CredentialID}, true
}
