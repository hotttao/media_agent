package library_object_upload

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"media_agent/media_materials/biz/shared/objectstorage"
)

var pngBytes = []byte("\x89PNG\r\n\x1a\nimage-data")

type fakeStorage struct {
	input objectstorage.PutInput
	err   error
}

func (s *fakeStorage) PutIfAbsent(_ context.Context, input objectstorage.PutInput) (objectstorage.ObjectInfo, error) {
	s.input = input
	if s.err != nil {
		return objectstorage.ObjectInfo{}, s.err
	}
	data, _ := io.ReadAll(input.Body)
	return objectstorage.ObjectInfo{ObjectPath: input.ObjectPath, SizeBytes: int64(len(data)), ContentType: input.ContentType, Checksum: input.Checksum}, nil
}

func validCommand() Command {
	return Command{OrganizationID: "org-1", Scope: "ai_clothing", Purpose: "material", CallerID: "user-1",
		IdempotencyKey: "upload-1", SizeBytes: int64(len(pngBytes)), Body: strings.NewReader(string(pngBytes))}
}

func TestUploadBuildsStableObjectPath(t *testing.T) {
	storage := &fakeStorage{}
	workflow := New(storage)
	first, err := workflow.Upload(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	secondCommand := validCommand()
	second, err := workflow.Upload(context.Background(), secondCommand)
	if err != nil {
		t.Fatalf("retry Upload: %v", err)
	}
	if first.ObjectPath != second.ObjectPath || !strings.HasPrefix(first.ObjectPath, "materials/org-1/ai_clothing/material/") || !strings.HasSuffix(first.ObjectPath, ".png") {
		t.Fatalf("unstable object paths: %q %q", first.ObjectPath, second.ObjectPath)
	}
	if storage.input.ContentType != "image/png" {
		t.Fatalf("content type=%q, want image/png", storage.input.ContentType)
	}
}

func TestUploadAcceptsLegacyFullBodyImageLargerThanFourMiB(t *testing.T) {
	data := append([]byte{}, pngBytes...)
	data = append(data, bytes.Repeat([]byte{0}, 9<<20)...)
	command := validCommand()
	command.Purpose = "virtual_ip_full_body"
	command.SizeBytes = int64(len(data))
	command.Body = bytes.NewReader(data)
	result, err := New(&fakeStorage{}).Upload(context.Background(), command)
	if err != nil {
		t.Fatalf("upload 9 MiB image: %v", err)
	}
	if result.SizeBytes != int64(len(data)) {
		t.Fatalf("stored %d bytes, want %d", result.SizeBytes, len(data))
	}
}

func TestUploadAcceptsMaterialVideoOnly(t *testing.T) {
	mp4 := []byte("\x00\x00\x00\x18ftypisom0000")
	command := validCommand()
	command.Body = bytes.NewReader(mp4)
	command.SizeBytes = int64(len(mp4))
	result, err := New(&fakeStorage{}).Upload(context.Background(), command)
	if err != nil || result.ContentType != "video/mp4" || !strings.HasSuffix(result.ObjectPath, ".mp4") {
		t.Fatalf("material mp4: result=%+v err=%v", result, err)
	}
	command.Purpose = "product_image"
	command.Body = bytes.NewReader(mp4)
	if _, err := New(&fakeStorage{}).Upload(context.Background(), command); !errors.Is(err, ErrInvalid) {
		t.Fatalf("product image accepted video: %v", err)
	}
}

func TestUploadRejectsInvalidInputsBeforeStorage(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Command)
		want error
	}{
		{"unsafe organization", func(c *Command) { c.OrganizationID = "../other" }, ErrInvalid},
		{"unknown scope", func(c *Command) { c.Scope = "unknown" }, ErrInvalid},
		{"unknown purpose", func(c *Command) { c.Purpose = "other" }, ErrInvalid},
		{"missing idempotency key", func(c *Command) { c.IdempotencyKey = "" }, ErrInvalid},
		{"too large", func(c *Command) { c.SizeBytes = MaxUploadBytes + 1 }, ErrTooLarge},
		{"not image", func(c *Command) { c.Body = strings.NewReader("plain text"); c.SizeBytes = 10 }, ErrInvalid},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			storage := &fakeStorage{}
			command := validCommand()
			test.edit(&command)
			_, err := New(storage).Upload(context.Background(), command)
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v, want %v", err, test.want)
			}
			if storage.input.ObjectPath != "" {
				t.Fatal("storage must not be called for invalid input")
			}
		})
	}
}

func TestUploadMapsStorageFailures(t *testing.T) {
	for _, test := range []struct {
		storageErr error
		want       error
	}{{objectstorage.ErrConflict, ErrConflict}, {objectstorage.ErrUnavailable, ErrUnavailable}} {
		_, err := New(&fakeStorage{err: test.storageErr}).Upload(context.Background(), validCommand())
		if !errors.Is(err, test.want) {
			t.Fatalf("error=%v, want %v", err, test.want)
		}
	}
}
