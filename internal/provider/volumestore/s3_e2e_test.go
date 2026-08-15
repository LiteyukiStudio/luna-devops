package volumestore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestS3StoreE2E(t *testing.T) {
	endpointValue := strings.TrimSpace(os.Getenv("VOLUME_E2E_S3_ENDPOINT"))
	if endpointValue == "" {
		t.Skip("set VOLUME_E2E_S3_ENDPOINT to run against a disposable S3-compatible service")
	}
	accessKey := strings.TrimSpace(os.Getenv("VOLUME_E2E_S3_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("VOLUME_E2E_S3_SECRET_KEY"))
	if accessKey == "" || secretKey == "" {
		t.Fatal("VOLUME_E2E_S3_ACCESS_KEY and VOLUME_E2E_S3_SECRET_KEY are required")
	}
	endpoint, err := url.Parse(endpointValue)
	if err != nil || endpoint.Host == "" {
		t.Fatalf("parse VOLUME_E2E_S3_ENDPOINT: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       endpoint.Scheme == "https",
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		t.Fatalf("create S3 administration client: %v", err)
	}
	bucket := fmt.Sprintf("luna-volume-e2e-%d", time.Now().UnixNano())
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatalf("create disposable bucket: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = client.RemoveObject(cleanupCtx, bucket, "transfers/vtx_e2e/archive", minio.RemoveObjectOptions{})
		_ = client.RemoveBucket(cleanupCtx, bucket)
	})

	store, err := NewS3Store(S3Config{
		Endpoint:                    endpointValue,
		Bucket:                      bucket,
		AccessKeyID:                 accessKey,
		SecretAccessKey:             secretKey,
		AllowInsecureEndpoint:       endpoint.Scheme == "http",
		DisableServerSideEncryption: true,
		PathStyle:                   true,
	})
	if err != nil {
		t.Fatalf("create S3 volume store: %v", err)
	}
	key := "transfers/vtx_e2e/archive"
	uploadID, err := store.CreateMultipart(ctx, key)
	if err != nil {
		t.Fatalf("create multipart upload: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = store.AbortMultipart(cleanupCtx, key, uploadID)
		_ = store.Delete(cleanupCtx, key)
	})

	first := bytes.Repeat([]byte("a"), 5*1024*1024)
	second := []byte("volume-transfer-tail")
	firstETag, err := store.WritePart(ctx, key, uploadID, 1, bytes.NewReader(first), int64(len(first)))
	if err != nil {
		t.Fatalf("write first multipart part: %v", err)
	}
	secondETag, err := store.WritePart(ctx, key, uploadID, 2, bytes.NewReader(second), int64(len(second)))
	if err != nil {
		t.Fatalf("write second multipart part: %v", err)
	}
	if err := store.CompleteMultipart(ctx, key, uploadID, []CompletedPart{
		{PartNumber: 1, ETag: firstETag},
		{PartNumber: 2, ETag: secondETag},
	}); err != nil {
		t.Fatalf("complete multipart upload: %v", err)
	}

	info, err := store.Head(ctx, key)
	if err != nil {
		t.Fatalf("head completed object: %v", err)
	}
	if want := int64(len(first) + len(second)); info.Size != want {
		t.Fatalf("completed object size = %d, want %d", info.Size, want)
	}
	body, err := store.ReadRange(ctx, key, int64(len(first)-2), 6)
	if err != nil {
		t.Fatalf("read cross-part range: %v", err)
	}
	content, readErr := io.ReadAll(body)
	closeErr := body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read range body: read=%v close=%v", readErr, closeErr)
	}
	if got, want := string(content), "aa"+string(second[:4]); got != want {
		t.Fatalf("range content = %q, want %q", got, want)
	}

	cancelledCtx, cancelRead := context.WithCancel(ctx)
	cancelRead()
	if _, err := store.ReadRange(cancelledCtx, key, 0, 1); err == nil {
		t.Fatal("cancelled range read unexpectedly succeeded")
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("delete completed object: %v", err)
	}
}
