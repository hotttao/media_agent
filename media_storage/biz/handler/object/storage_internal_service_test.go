package object

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	domain "media_agent/media_storage/biz/domain/stored_object"
	storedobjectsvc "media_agent/media_storage/biz/service/stored_object"
)

type handlerDriver struct{}

func (handlerDriver) Put(_ context.Context, objectPath string, body io.Reader) (domain.BlobInfo, error) {
	data, _ := io.ReadAll(body)
	return domain.BlobInfo{ObjectPath: objectPath, SizeBytes: int64(len(data)), Checksum: "sha256:test"}, nil
}
func (handlerDriver) Open(context.Context, string) (io.ReadCloser, error) { return nil, nil }
func (handlerDriver) Stat(context.Context, string) (domain.BlobInfo, error) {
	return domain.BlobInfo{}, nil
}
func (handlerDriver) Delete(context.Context, string) error { return nil }

type handlerRepo struct{}

func (handlerRepo) SaveActive(_ context.Context, object domain.StoredObject) (domain.StoredObject, error) {
	return object, nil
}

func TestAuthorizeInternalFailsClosed(t *testing.T) {
	previous := InternalToken
	t.Cleanup(func() { InternalToken = previous })
	InternalToken = "internal-secret"

	without := app.NewContext(0)
	if authorizeInternal(without) || without.Response.StatusCode() != 401 {
		t.Fatal("missing internal token must be rejected")
	}
	wrong := app.NewContext(0)
	wrong.Request.Header.Set("X-Media-Storage-Internal-Token", "wrong")
	if authorizeInternal(wrong) {
		t.Fatal("wrong internal token must be rejected")
	}
	valid := app.NewContext(0)
	valid.Request.Header.Set("X-Media-Storage-Internal-Token", "internal-secret")
	if !authorizeInternal(valid) {
		t.Fatal("matching internal token must be accepted")
	}
}

func TestAuthorizeInternalRejectsUnsetConfiguration(t *testing.T) {
	previous := InternalToken
	t.Cleanup(func() { InternalToken = previous })
	InternalToken = ""
	c := app.NewContext(0)
	c.Request.Header.Set("X-Media-Storage-Internal-Token", "anything")
	if authorizeInternal(c) {
		t.Fatal("unset internal token configuration must fail closed")
	}
}

func TestPutObjectBindsQueryWithRawImageBody(t *testing.T) {
	previousToken, previousService := InternalToken, Svc
	t.Cleanup(func() { InternalToken, Svc = previousToken, previousService })
	InternalToken = "internal-secret"
	Svc = storedobjectsvc.NewService(handlerDriver{}, handlerRepo{}, "local_file")
	h := server.Default()
	h.PUT("/api/internal/v1/objects", PutObject)
	response := ut.PerformRequest(h.Engine, "PUT", "/api/internal/v1/objects?object_path=materials/org/file.png",
		&ut.Body{Body: strings.NewReader("png"), Len: 3},
		ut.Header{Key: "Content-Type", Value: "image/png"},
		ut.Header{Key: "X-Media-Storage-Internal-Token", Value: "internal-secret"})
	if got := response.Result().StatusCode(); got != 200 {
		t.Fatalf("status=%d body=%s", got, response.Result().Body())
	}
}
