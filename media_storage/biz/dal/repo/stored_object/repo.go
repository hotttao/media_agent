package stored_object

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"media_agent/media_storage/biz/dal"
	"media_agent/media_storage/biz/dal/model"
	domain "media_agent/media_storage/biz/domain/stored_object"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) SaveActive(ctx context.Context, object domain.StoredObject) (domain.StoredObject, error) {
	if r == nil || r.db == nil {
		return domain.StoredObject{}, domain.ErrStorageUnavailable
	}
	row := model.StoredObject{
		ObjectPath: object.ObjectPath, Namespace: object.Namespace, Driver: object.Driver,
		BackendKey: object.BackendKey, ContentType: object.ContentType, SizeBytes: object.SizeBytes,
		Checksum: object.Checksum, State: "active",
	}
	db := dal.FromContext(ctx, r.db)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return domain.StoredObject{}, err
	}
	var existing model.StoredObject
	if err := db.Where("object_path = ?", object.ObjectPath).First(&existing).Error; err != nil {
		return domain.StoredObject{}, err
	}
	if existing.State != "active" || existing.SizeBytes != object.SizeBytes || existing.Checksum != object.Checksum || existing.ContentType != object.ContentType {
		return domain.StoredObject{}, domain.ErrObjectConflict
	}
	if existing.Namespace != object.Namespace || existing.Driver != object.Driver || existing.BackendKey != object.BackendKey {
		return domain.StoredObject{}, errors.New("stored object metadata mismatch")
	}
	return toDomain(existing), nil
}

func toDomain(row model.StoredObject) domain.StoredObject {
	return domain.StoredObject{ObjectPath: row.ObjectPath, Namespace: row.Namespace, Driver: row.Driver,
		BackendKey: row.BackendKey, ContentType: row.ContentType, SizeBytes: row.SizeBytes, Checksum: row.Checksum}
}
