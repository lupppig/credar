package objectstorage_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/lupppig/credar/backend/internal/platform/objectstorage"
)

func TestStoreRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := objectstorage.New(objectstorage.Config{Endpoint: "ftp://objects.example"}); err == nil {
		t.Fatal("expected invalid object-storage configuration to fail")
	}
}

func TestStoreRequiresOpaqueKeysAndStartup(t *testing.T) {
	t.Parallel()

	store, err := objectstorage.New(objectstorage.Config{
		Endpoint:  "http://127.0.0.1:8333",
		Bucket:    "test-bucket",
		AccessKey: "access",
		SecretKey: "secret",
	})
	if err != nil {
		t.Fatalf("construct object storage: %v", err)
	}
	if err := store.Put(context.Background(), "customer-name.pdf", bytes.NewReader(nil), 0, "application/pdf"); err == nil {
		t.Fatal("expected descriptive object key to fail")
	}
	key, err := objectstorage.NewKey()
	if err != nil {
		t.Fatalf("create opaque key: %v", err)
	}
	if err := store.Put(context.Background(), key, bytes.NewReader(nil), 0, "application/octet-stream"); err == nil {
		t.Fatal("expected unstarted store to fail")
	}
}
