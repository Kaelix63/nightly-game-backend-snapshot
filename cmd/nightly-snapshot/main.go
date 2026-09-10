package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	snapshotstore "github.com/infrai-examples/nightly-game-snapshot"
)

func main() {
	var snapshotPath string
	var bucket string
	flag.StringVar(&snapshotPath, "file", "", "path to an atomically exported game backend JSON snapshot")
	flag.StringVar(&bucket, "bucket", "game-backups", "destination bucket")
	flag.Parse()

	if snapshotPath == "" {
		log.Fatal("-file is required")
	}
	snapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		log.Fatalf("read snapshot: %v", err)
	}

	client, err := snapshotstore.NewClient(os.Getenv("INFRAI_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := client.CreateBucket(ctx, bucket); err != nil {
		log.Fatalf("initialize backup bucket: %v", err)
	}

	night := time.Now().UTC().Format("2006-01-02")
	objectKey := fmt.Sprintf("game-backend-%s%s", night, filepath.Ext(snapshotPath))
	digest := sha256.Sum256([]byte(bucket + "\x00" + objectKey))
	idempotencyKey := "nightly-game-snapshot-" + hex.EncodeToString(digest[:16])
	presigned, err := client.PresignSnapshotPut(ctx, bucket, objectKey, int64(len(snapshot)), idempotencyKey)
	if err != nil {
		log.Fatalf("presign snapshot upload: %v", err)
	}
	if err := client.UploadSnapshot(ctx, presigned.URL, snapshot); err != nil {
		log.Fatal(err)
	}

	log.Printf("snapshot stored: bucket=%s key=%s bytes=%d", bucket, objectKey, len(snapshot))
}
