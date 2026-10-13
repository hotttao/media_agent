package agent_instance

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"gorm.io/gorm"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db} }

func (r *Repository) VerifyMetadataInstance(ctx context.Context, ws, config, replica, incarnation, credential string) error {
	if len(credential) < 16 || len(credential) > 1024 {
		return client.ErrMetadataInstanceUnverified
	}
	var row struct{ CredentialHash string }
	err := r.db.WithContext(ctx).Table("agent_instances").Where("workspace_id=? AND agent_config_id=? AND replica_id=? AND incarnation_id=?", ws, config, replica, incarnation).Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return client.ErrMetadataInstanceUnverified
	}
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(credential))
	if row.CredentialHash == "" || subtle.ConstantTimeCompare([]byte(row.CredentialHash), []byte(hex.EncodeToString(hash[:]))) != 1 {
		return client.ErrMetadataInstanceUnverified
	}
	return nil
}

func (r *Repository) ListMetadataInstances(ctx context.Context, ws string) ([]client.MetadataInstance, error) {
	var rows []struct{ WorkspaceID, AgentConfigID, ReplicaID, IncarnationID, Admission string }
	if err := r.db.WithContext(ctx).Table("agent_instances").Where("workspace_id=?", ws).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []client.MetadataInstance{}
	for _, v := range rows {
		out = append(out, client.MetadataInstance{WorkspaceID: v.WorkspaceID, AgentConfigID: v.AgentConfigID, ReplicaID: v.ReplicaID, IncarnationID: v.IncarnationID, Admission: v.Admission})
	}
	return out, nil
}
