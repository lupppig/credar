// Package objectstorage provides an application-owned boundary over the local
// S3-compatible SeaweedFS service.
package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/s3utils"
)

// ErrNotFound reports an absent object without leaking provider error types.
var ErrNotFound = errors.New("object not found")

// Config contains one isolated object-storage bucket configuration.
type Config struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
}

// Object contains the readable bytes and provider-neutral metadata.
type Object struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	ETag        string
}

// Store owns the bucket lifecycle and opaque object operations.
type Store struct {
	configuration Config
	endpoint      string
	secure        bool

	mu     sync.RWMutex
	client *minio.Client
}

// New validates settings without opening a network connection.
func New(configuration Config) (*Store, error) {
	parsed, err := url.Parse(configuration.Endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("object-storage endpoint must be an HTTP URL")
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("object-storage endpoint must not contain credentials, a path, query, or fragment")
	}
	if err := s3utils.CheckValidBucketNameStrict(configuration.Bucket); err != nil {
		return nil, fmt.Errorf("object-storage bucket is invalid: %w", err)
	}
	if configuration.AccessKey == "" || configuration.SecretKey == "" {
		return nil, fmt.Errorf("object-storage credentials are required")
	}
	return &Store{
		configuration: configuration,
		endpoint:      parsed.Host,
		secure:        parsed.Scheme == "https",
	}, nil
}

// Name identifies this dependency in readiness output.
func (*Store) Name() string {
	return "object-storage"
}

// Start connects to SeaweedFS and creates the configured bucket when absent.
func (store *Store) Start(ctx context.Context) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.client != nil {
		return fmt.Errorf("object storage has already started")
	}
	client, err := minio.New(store.endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(store.configuration.AccessKey, store.configuration.SecretKey, ""),
		Secure: store.secure,
	})
	if err != nil {
		return fmt.Errorf("construct object-storage client: %w", err)
	}
	exists, err := client.BucketExists(ctx, store.configuration.Bucket)
	if err != nil {
		return fmt.Errorf("inspect object-storage bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, store.configuration.Bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("create object-storage bucket: %w", err)
		}
	}
	store.client = client
	return nil
}

// Check verifies connectivity and the required bucket without mutating it.
func (store *Store) Check(ctx context.Context) error {
	client, err := store.startedClient()
	if err != nil {
		return err
	}
	exists, err := client.BucketExists(ctx, store.configuration.Bucket)
	if err != nil {
		return fmt.Errorf("inspect object-storage bucket: %w", err)
	}
	if !exists {
		return fmt.Errorf("object-storage bucket is missing")
	}
	return nil
}

// Close releases this process's reference to the stateless S3 client.
func (store *Store) Close(context.Context) error {
	store.mu.Lock()
	store.client = nil
	store.mu.Unlock()
	return nil
}

// NewKey returns an opaque provider-independent object key.
func NewKey() (string, error) {
	key, err := identifier.NewUUIDv7()
	if err != nil {
		return "", fmt.Errorf("create opaque object key: %w", err)
	}
	return key, nil
}

// Put stores bytes under an opaque key. Content validation and size policy
// belong to the future owning business module.
func (store *Store) Put(
	ctx context.Context,
	key string,
	body io.Reader,
	size int64,
	contentType string,
) error {
	if err := validateObjectInput(key, body, size, contentType); err != nil {
		return err
	}
	client, err := store.startedClient()
	if err != nil {
		return err
	}
	if _, err := client.PutObject(
		ctx,
		store.configuration.Bucket,
		key,
		body,
		size,
		minio.PutObjectOptions{ContentType: contentType},
	); err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

// Get opens an opaque object and returns provider-neutral metadata.
func (store *Store) Get(ctx context.Context, key string) (Object, error) {
	if !identifier.IsUUIDv7(key) {
		return Object{}, fmt.Errorf("object key must be a UUIDv7")
	}
	client, err := store.startedClient()
	if err != nil {
		return Object{}, err
	}
	information, err := client.StatObject(ctx, store.configuration.Bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return Object{}, normalizeError(err)
	}
	body, err := client.GetObject(ctx, store.configuration.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return Object{}, normalizeError(err)
	}
	return Object{
		Body:        body,
		Size:        information.Size,
		ContentType: information.ContentType,
		ETag:        information.ETag,
	}, nil
}

// Delete removes an opaque object. Deletion and retention authorization belong
// to the future owning business module.
func (store *Store) Delete(ctx context.Context, key string) error {
	if !identifier.IsUUIDv7(key) {
		return fmt.Errorf("object key must be a UUIDv7")
	}
	client, err := store.startedClient()
	if err != nil {
		return err
	}
	if err := client.RemoveObject(ctx, store.configuration.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return normalizeError(err)
	}
	return nil
}

func (store *Store) startedClient() (*minio.Client, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.client == nil {
		return nil, fmt.Errorf("object storage is not started")
	}
	return store.client, nil
}

func validateObjectInput(key string, body io.Reader, size int64, contentType string) error {
	if !identifier.IsUUIDv7(key) {
		return fmt.Errorf("object key must be a UUIDv7")
	}
	if body == nil {
		return fmt.Errorf("object body is required")
	}
	if size < 0 {
		return fmt.Errorf("object size must not be negative")
	}
	if strings.TrimSpace(contentType) == "" {
		return fmt.Errorf("object content type is required")
	}
	return nil
}

func normalizeError(err error) error {
	response := minio.ToErrorResponse(err)
	switch response.Code {
	case "NoSuchKey", "NoSuchObject", "NoSuchBucket", "NotFound":
		return ErrNotFound
	default:
		return fmt.Errorf("object-storage operation: %w", err)
	}
}
