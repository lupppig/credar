//go:build integration

package objectstorage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/lupppig/credar/backend/internal/platform/objectstorage"
)

func TestOpaqueObjectRoundTrip(t *testing.T) {
	endpoint := os.Getenv("CREDAR_TEST_OBJECT_STORAGE_ENDPOINT")
	bucket := os.Getenv("CREDAR_TEST_OBJECT_STORAGE_BUCKET")
	accessKey := os.Getenv("CREDAR_TEST_OBJECT_STORAGE_ACCESS_KEY")
	secretKey := os.Getenv("CREDAR_TEST_OBJECT_STORAGE_SECRET_KEY")
	if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		t.Skip("object-storage integration configuration is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := objectstorage.New(objectstorage.Config{
		Endpoint:  endpoint,
		Bucket:    bucket,
		AccessKey: accessKey,
		SecretKey: secretKey,
	})
	if err != nil {
		t.Fatalf("construct object storage: %v", err)
	}
	if err := store.Start(ctx); err != nil {
		t.Fatalf("start object storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close(context.Background()) })
	if err := store.Check(ctx); err != nil {
		t.Fatalf("check object storage: %v", err)
	}

	key, err := objectstorage.NewKey()
	if err != nil {
		t.Fatalf("create opaque key: %v", err)
	}
	want := []byte("opaque-object-round-trip")
	if err := store.Put(ctx, key, bytes.NewReader(want), int64(len(want)), "application/octet-stream"); err != nil {
		t.Fatalf("put object: %v", err)
	}
	object, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	got, err := io.ReadAll(object.Body)
	closeErr := object.Body.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("read object: %v; close: %v", err, closeErr)
	}
	if !bytes.Equal(got, want) || object.Size != int64(len(want)) || object.ContentType != "application/octet-stream" {
		t.Fatalf("unexpected object round trip: size=%d type=%q body=%q", object.Size, object.ContentType, got)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("delete object: %v", err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, objectstorage.ErrNotFound) {
		t.Fatalf("expected deleted object to be absent, got %v", err)
	}
}
