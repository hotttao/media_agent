package objectstorage

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type UploadInput struct {
	ObjectPath, ContentType, Checksum, ContentMD5 string
	SizeBytes                                     int64
}

type UploadGrant struct {
	ObjectPath      string
	URL             string
	Headers         map[string]string
	ExpiresAt       int64
	AlreadyUploaded bool
}

// PresignPut delegates bytes to the provider. Each address is conditionally
// write-once and bound to payload checksums; it never grants bucket credentials.
func (c *Client) PresignPut(ctx context.Context, input UploadInput) (UploadGrant, error) {
	sha, shaErr := hex.DecodeString(input.Checksum)
	md5, md5Err := base64.StdEncoding.DecodeString(input.ContentMD5)
	if c == nil || c.presigner == nil || input.ObjectPath == "" || input.ContentType == "" || input.SizeBytes <= 0 ||
		shaErr != nil || len(sha) != 32 || strings.ToLower(input.Checksum) != input.Checksum || md5Err != nil || len(md5) != 16 || base64.StdEncoding.EncodeToString(md5) != input.ContentMD5 {
		return UploadGrant{}, ErrUnavailable
	}
	if existing, err := c.Stat(ctx, input.ObjectPath); err == nil {
		if existing.Checksum != input.Checksum || existing.SizeBytes != input.SizeBytes || existing.ContentType != input.ContentType {
			return UploadGrant{}, ErrConflict
		}
		return UploadGrant{ObjectPath: input.ObjectPath, AlreadyUploaded: true}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return UploadGrant{}, ErrUnavailable
	}
	request, err := c.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket), Key: aws.String(input.ObjectPath), ContentLength: aws.Int64(input.SizeBytes),
		ContentType: aws.String(input.ContentType), ContentMD5: aws.String(input.ContentMD5),
		ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(sha)), IfNoneMatch: aws.String("*"),
		Metadata: map[string]string{checksumMetadataKey: input.Checksum},
	}, s3.WithPresignExpires(c.ttl))
	if err != nil {
		return UploadGrant{}, ErrUnavailable
	}
	headers := map[string]string{}
	for key, values := range request.SignedHeader {
		headers[key] = strings.Join(values, ",")
	}
	return UploadGrant{ObjectPath: input.ObjectPath, URL: request.URL, Headers: headers, ExpiresAt: time.Now().Add(c.ttl).Unix()}, nil
}
