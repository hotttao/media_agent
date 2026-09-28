package apitoken

import (
	"context"

	client "media_agent/media_auth/biz/shared/client/talos"
)

type TalosAdapter struct{ Client *client.Client }

func (a TalosAdapter) Issue(ctx context.Context, cmd IssueSecretCommand) (IssuedSecret, error) {
	result, err := a.Client.Issue(ctx, client.IssueCommand{
		Name: cmd.Name, ActorID: cmd.ActorID, RequestID: cmd.RequestID, TTL: cmd.TTL, Scopes: cmd.Scopes, Metadata: cmd.Metadata,
	})
	if err != nil {
		return IssuedSecret{}, err
	}
	return IssuedSecret{KeyID: result.KeyID, ActorID: result.ActorID, Secret: result.Secret, Scopes: result.Scopes, ExpiresAt: result.ExpiresAt}, nil
}

func (a TalosAdapter) Verify(ctx context.Context, secret string) (VerifiedSecret, error) {
	result, err := a.Client.Verify(ctx, secret)
	if err != nil {
		return VerifiedSecret{}, err
	}
	return VerifiedSecret{KeyID: result.KeyID, ActorID: result.ActorID, Scopes: result.Scopes, ExpiresAt: result.ExpiresAt}, nil
}

func (a TalosAdapter) Rotate(ctx context.Context, keyID string, scopes []string) (IssuedSecret, error) {
	result, err := a.Client.Rotate(ctx, keyID, scopes)
	if err != nil {
		return IssuedSecret{}, err
	}
	return IssuedSecret{KeyID: result.KeyID, ActorID: result.ActorID, Secret: result.Secret, Scopes: result.Scopes, ExpiresAt: result.ExpiresAt}, nil
}

func (a TalosAdapter) Revoke(ctx context.Context, keyID string) error {
	return a.Client.Revoke(ctx, keyID)
}

func (a TalosAdapter) Get(ctx context.Context, keyID string) (SecretMetadata, error) {
	result, err := a.Client.Get(ctx, keyID)
	return SecretMetadata{KeyID: result.KeyID, Status: result.Status}, err
}
