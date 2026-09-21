package stored_object

import (
	"context"
	"io"
	"strings"

	domain "media_agent/media_storage/biz/domain/stored_object"
)

type Service struct {
	driver     domain.Driver
	repository domain.Repository
	driverName string
}

func NewService(driver domain.Driver, repository domain.Repository, driverName string) *Service {
	return &Service{driver: driver, repository: repository, driverName: driverName}
}

func (s *Service) Put(ctx context.Context, objectPath, contentType string, body io.Reader) (domain.StoredObject, error) {
	if s == nil || s.driver == nil || s.repository == nil {
		return domain.StoredObject{}, domain.ErrStorageUnavailable
	}
	if body == nil || strings.TrimSpace(contentType) == "" {
		return domain.StoredObject{}, domain.ErrInvalidObjectPath
	}
	parts := strings.Split(objectPath, "/")
	if len(parts) < 2 {
		return domain.StoredObject{}, domain.ErrInvalidObjectPath
	}
	if parts[0] != "materials" {
		return domain.StoredObject{}, domain.ErrNamespaceForbidden
	}
	blob, err := s.driver.Put(ctx, objectPath, body)
	if err != nil {
		return domain.StoredObject{}, err
	}
	return s.repository.SaveActive(ctx, domain.StoredObject{
		ObjectPath: blob.ObjectPath, Namespace: parts[0], Driver: s.driverName,
		BackendKey: blob.ObjectPath, ContentType: contentType, SizeBytes: blob.SizeBytes, Checksum: blob.Checksum,
	})
}
