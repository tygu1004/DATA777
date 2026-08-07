package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3 reads objects from any S3-compatible bucket (AWS S3, RustFS, MinIO, GCS interop, R2, ...).
// Credentials are resolved through the standard AWS SDK chain (env vars, shared config,
// IAM role), so no credential flags are needed here.
type S3 struct {
	client *s3.Client
	bucket string
}

type S3Config struct {
	Bucket    string
	Region    string
	Endpoint  string // non-AWS endpoint, e.g. http://localhost:9000 for RustFS/MinIO; empty uses real AWS S3
	PathStyle bool   // required by most self-hosted S3-compatible servers (RustFS/MinIO default to this)
}

func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.PathStyle
	})

	return &S3{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3) Walk(ctx context.Context, root string, fn func(path string, size int64) error) error {
	prefix := strings.TrimPrefix(root, "/")
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: &prefix,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range page.Contents {
			if err := fn(aws.ToString(obj.Key), aws.ToInt64(obj.Size)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *S3) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &path,
	})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", path, err)
	}
	return out.Body, nil
}
