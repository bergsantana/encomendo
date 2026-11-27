//go:build integration

// Package storage_test runs the S3 photo store against a real Silo (the
// MinIO fork) in a testcontainers container (ADR 0024): signed uploads with net/http, the
// confirm checks through the catalog use cases, and the store's answers.
package storage_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// minioImage is the S3 server the tests run, the same one
// deploy/docker-compose.yml runs: Silo, the maintained MinIO fork,
// RELEASE.2026-11-12T00-00-00Z, pinned by digest (ADR 0024, amended).
const minioImage = "pgsty/silo@sha256:c20d112390b9a9ce8c11b8af95ad35bcaa62f4759663c09c936cd8d2b511f329"

const (
	accessKey = "encomendo"
	secretKey = "encomendo_minio_test"
	bucket    = "encomendo-photos"
)

// endpoint is the container's host:port.
var endpoint string

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        minioImage,
			Cmd:          []string{"silo", "server", "/data"},
			Env:          map[string]string{"MINIO_ROOT_USER": accessKey, "MINIO_ROOT_PASSWORD": secretKey},
			ExposedPorts: []string{"9000/tcp"},
			WaitingFor:   wait.ForHTTP("/minio/health/live").WithPort("9000/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "start minio:", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(ctr) }() // the reaper removes it if this fails
	host, err := ctr.Host(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	port, err := ctr.MappedPort(ctx, "9000/tcp")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	endpoint = host + ":" + port.Port()
	if err := makeBucket(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "make bucket:", err)
		return 1
	}
	return m.Run()
}

// makeBucket creates the bucket, as the compose bucket-init job does.
func makeBucket(ctx context.Context) error {
	c, err := rawClient()
	if err != nil {
		return err
	}
	return c.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"})
}

// rawClient is a plain client, for checking what the adapter really stored.
func rawClient() (*minio.Client, error) {
	return minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath,
	})
}
