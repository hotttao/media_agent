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

type RotationKey struct {
	KeyID, ActorID, Status string
	Metadata               map[string]any
}

type RotationKeyPage struct {
	Keys          []RotationKey
	NextPageToken string
}

func (a TalosAdapter) List(ctx context.Context, actorID, pageToken string) (RotationKeyPage, error) {
	page, err := a.Client.List(ctx, actorID, pageToken)
	if err != nil {
		return RotationKeyPage{}, err
	}
	result := RotationKeyPage{Keys: make([]RotationKey, 0, len(page.Keys)), NextPageToken: page.NextPageToken}
	for _, key := range page.Keys {
		result.Keys = append(result.Keys, RotationKey{KeyID: key.KeyID, ActorID: key.ActorID, Status: key.Status, Metadata: key.Metadata})
	}
	return result, nil
}

func (a TalosAdapter) RotateWithMetadata(ctx context.Context, keyID string, scopes []string, metadata map[string]any) (IssuedSecret, error) {
	result, err := a.Client.RotateWithMetadata(ctx, keyID, scopes, metadata)
	if err != nil {
		return IssuedSecret{}, err
	}
	return IssuedSecret{KeyID: result.KeyID, ActorID: result.ActorID, Secret: result.Secret, Scopes: result.Scopes, ExpiresAt: result.ExpiresAt}, nil
}
