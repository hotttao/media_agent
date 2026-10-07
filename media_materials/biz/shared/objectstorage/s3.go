package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

var (
	ErrConflict    = errors.New("stored object conflict")
	ErrNotFound    = errors.New("stored object not found")
	ErrUnavailable = errors.New("object storage unavailable")
)

const checksumMetadataKey = "checksum-sha256"

type Config struct {
	Provider       string
	Endpoint       string
	PublicEndpoint string
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool
	PresignTTL     time.Duration
}

type PutInput struct {
	ObjectPath  string
	ContentType string
	Checksum    string
	SizeBytes   int64
	Body        io.Reader
}

type ObjectInfo struct {
	ObjectPath  string
	SizeBytes   int64
	ContentType string
	Checksum    string
	ETag        string
}

type ResolvedObject struct {
	URL         string
	ExpiresAt   int64
	ContentType string
	SizeBytes   int64
}

type Client struct {
	bucket    string
	ttl       time.Duration
	api       *s3.Client
	presigner *s3.PresignClient
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.Provider != "s3" || cfg.Bucket == "" || cfg.Region == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("incomplete S3 object storage configuration")
	}
	if cfg.PresignTTL <= 0 || cfg.PresignTTL > 5*time.Minute {
		cfg.PresignTTL = 5 * time.Minute
	}
	loaded, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load S3 configuration: %w", err)
	}
	api := newS3Client(loaded, cfg.Endpoint, cfg.ForcePathStyle)
	publicEndpoint := cfg.PublicEndpoint
	if publicEndpoint == "" {
		publicEndpoint = cfg.Endpoint
	}
	presignAPI := newS3Client(loaded, publicEndpoint, cfg.ForcePathStyle)
	return &Client{bucket: cfg.Bucket, ttl: cfg.PresignTTL, api: api, presigner: s3.NewPresignClient(presignAPI)}, nil
}

func newS3Client(cfg aws.Config, endpoint string, forcePathStyle bool) *s3.Client {
	return s3.NewFromConfig(cfg, func(options *s3.Options) {
		if endpoint != "" {
			options.BaseEndpoint = aws.String(strings.TrimRight(endpoint, "/"))
		}
		options.UsePathStyle = forcePathStyle
	})
}

func (c *Client) PutIfAbsent(ctx context.Context, input PutInput) (ObjectInfo, error) {
	if c == nil || c.api == nil || input.ObjectPath == "" || input.ContentType == "" || input.Checksum == "" || input.SizeBytes < 0 || input.Body == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	if existing, err := c.Stat(ctx, input.ObjectPath); err == nil {
		return compareExisting(existing, input.Checksum)
	} else if !errors.Is(err, ErrNotFound) {
		return ObjectInfo{}, err
	}

	result, err := c.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(input.ObjectPath),
		Body:          input.Body,
		ContentLength: aws.Int64(input.SizeBytes),
		ContentType:   aws.String(input.ContentType),
		IfNoneMatch:   aws.String("*"),
		Metadata:      map[string]string{checksumMetadataKey: input.Checksum},
	})
	if err != nil {
		if isPreconditionFailed(err) {
			existing, statErr := c.Stat(ctx, input.ObjectPath)
			if statErr != nil {
				return ObjectInfo{}, statErr
			}
			return compareExisting(existing, input.Checksum)
		}
		return ObjectInfo{}, fmt.Errorf("%w: put object: %v", ErrUnavailable, err)
	}
	return ObjectInfo{ObjectPath: input.ObjectPath, SizeBytes: input.SizeBytes, ContentType: input.ContentType,
		Checksum: input.Checksum, ETag: aws.ToString(result.ETag)}, nil
}

func compareExisting(existing ObjectInfo, checksum string) (ObjectInfo, error) {
	if existing.Checksum != checksum {
		return ObjectInfo{}, ErrConflict
	}
	return existing, nil
}

func (c *Client) Stat(ctx context.Context, objectPath string) (ObjectInfo, error) {
	if c == nil || c.api == nil || objectPath == "" {
		return ObjectInfo{}, ErrUnavailable
	}
	result, err := c.api.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(objectPath)})
	if err != nil {
		if isNotFound(err) {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, fmt.Errorf("%w: head object: %v", ErrUnavailable, err)
	}
	return ObjectInfo{ObjectPath: objectPath, SizeBytes: aws.ToInt64(result.ContentLength),
		ContentType: aws.ToString(result.ContentType), Checksum: result.Metadata[checksumMetadataKey], ETag: aws.ToString(result.ETag)}, nil
}

func (c *Client) PresignGet(ctx context.Context, objectPath string) (ResolvedObject, error) {
	info, err := c.Stat(ctx, objectPath)
	if err != nil {
		return ResolvedObject{}, err
	}
	result, err := c.presigner.PresignGetObject(ctx,
		&s3.GetObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(objectPath)},
		s3.WithPresignExpires(c.ttl),
	)
	if err != nil {
		return ResolvedObject{}, fmt.Errorf("%w: presign object: %v", ErrUnavailable, err)
	}
	return ResolvedObject{URL: result.URL, ExpiresAt: time.Now().Add(c.ttl).Unix(), ContentType: info.ContentType, SizeBytes: info.SizeBytes}, nil
}

func (c *Client) Delete(ctx context.Context, objectPath string) error {
	if c == nil || c.api == nil || objectPath == "" {
		return ErrUnavailable
	}
	_, err := c.api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(objectPath)})
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("%w: delete object: %v", ErrUnavailable, err)
	}
	return nil
}

func isNotFound(err error) bool {
	return hasAPIErrorCode(err, "NotFound", "NoSuchKey", "NoSuchBucket") || hasHTTPStatus(err, 404)
}

func isPreconditionFailed(err error) bool {
	return hasAPIErrorCode(err, "PreconditionFailed", "ConditionalRequestConflict") || hasHTTPStatus(err, 409) || hasHTTPStatus(err, 412)
}

func hasAPIErrorCode(err error, codes ...string) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	for _, code := range codes {
		if apiErr.ErrorCode() == code {
			return true
		}
	}
	return false
}

func hasHTTPStatus(err error, status int) bool {
	var responseErr *awshttp.ResponseError
	return errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == status
}
