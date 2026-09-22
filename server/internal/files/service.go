package files

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	ErrInvalidSignature = errors.New("invalid signed URL")
	ErrExpiredSignature = errors.New("signed URL expired")
	ErrNotFound         = errors.New("file not found")
)

type Service struct {
	baseURL string
	secret  []byte
	ttl     time.Duration

	mu    sync.RWMutex
	blobs map[string][]byte

	s3     *minio.Client
	bucket string
	prefix string
}

type S3Config struct {
	Endpoint   string
	Bucket     string
	AccessKey  string
	SecretKey  string
	Region     string
	Prefix     string
	TTL        time.Duration
	AutoCreate bool
}

func NewService(baseURL, secret string, ttl time.Duration) *Service {
	return &Service{
		baseURL: baseURL,
		secret:  []byte(secret),
		ttl:     ttl,
		blobs:   make(map[string][]byte),
	}
}

func NewS3Service(ctx context.Context, cfg S3Config) (*Service, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" ||
		cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("object storage endpoint, bucket and credentials are required")
	}
	if cfg.TTL <= 0 {
		return nil, errors.New("object storage URL TTL must be positive")
	}

	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil || endpoint.Host == "" {
		return nil, errors.New("invalid object storage endpoint")
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, errors.New("object storage endpoint must use http or https")
	}

	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: endpoint.Scheme == "https",
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check object storage bucket: %w", err)
	}
	if !exists {
		if !cfg.AutoCreate {
			return nil, fmt.Errorf("object storage bucket %q does not exist", cfg.Bucket)
		}
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{
			Region: cfg.Region,
		}); err != nil {
			return nil, fmt.Errorf("create object storage bucket: %w", err)
		}
	}

	return &Service{
		ttl:    cfg.TTL,
		s3:     client,
		bucket: cfg.Bucket,
		prefix: strings.Trim(strings.TrimSpace(cfg.Prefix), "/"),
	}, nil
}

func (s *Service) IsLocal() bool {
	return s.s3 == nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.s3 == nil {
		return nil
	}

	exists, err := s.s3.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check object storage health: %w", err)
	}
	if !exists {
		return fmt.Errorf("object storage bucket %q is unavailable", s.bucket)
	}
	return nil
}

func (s *Service) UploadURL(
	ctx context.Context,
	transferID,
	checksum string,
) (string, error) {
	if s.s3 == nil {
		return s.signedURL("upload", transferID, "/dev-upload/"+transferID), nil
	}

	headers := make(http.Header)
	headers.Set("X-Amz-Meta-Sha256", strings.ToLower(checksum))
	u, err := s.s3.PresignHeader(
		ctx,
		http.MethodPut,
		s.bucket,
		s.objectKey(transferID),
		s.ttl,
		nil,
		headers,
	)
	if err != nil {
		return "", fmt.Errorf("presign upload: %w", err)
	}
	return u.String(), nil
}

func (s *Service) DownloadURL(
	ctx context.Context,
	transferID string,
) (string, error) {
	if s.s3 == nil {
		return s.signedURL("download", transferID, "/dev-download/"+transferID), nil
	}

	u, err := s.s3.PresignedGetObject(
		ctx,
		s.bucket,
		s.objectKey(transferID),
		s.ttl,
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("presign download: %w", err)
	}
	return u.String(), nil
}

func (s *Service) InspectUploaded(
	ctx context.Context,
	transferID string,
) (int64, string, error) {
	if s.s3 == nil {
		data, err := s.Get(transferID)
		if err != nil {
			return 0, "", fs.ErrNotExist
		}
		sum := sha256.Sum256(data)
		return int64(len(data)), hex.EncodeToString(sum[:]), nil
	}

	info, err := s.s3.StatObject(
		ctx,
		s.bucket,
		s.objectKey(transferID),
		minio.StatObjectOptions{},
	)
	if err != nil {
		response := minio.ToErrorResponse(err)
		if response.StatusCode == http.StatusNotFound ||
			response.Code == "NoSuchKey" ||
			response.Code == "NoSuchObject" {
			return 0, "", fs.ErrNotExist
		}
		return 0, "", fmt.Errorf("stat uploaded object: %w", err)
	}

	checksum := info.Metadata.Get("X-Amz-Meta-Sha256")
	if checksum == "" {
		for key, value := range info.UserMetadata {
			if strings.EqualFold(key, "sha256") {
				checksum = value
				break
			}
		}
	}

	return info.Size, strings.ToLower(strings.TrimSpace(checksum)), nil
}

func (s *Service) Verify(action, transferID, expires, signature string) error {
	expUnix, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return ErrInvalidSignature
	}
	if time.Now().Unix() > expUnix {
		return ErrExpiredSignature
	}

	expected := s.signature(action, transferID, expUnix)
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return ErrInvalidSignature
	}
	expectedBytes, _ := hex.DecodeString(expected)
	if !hmac.Equal(provided, expectedBytes) {
		return ErrInvalidSignature
	}
	return nil
}

func (s *Service) Put(transferID string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobs[transferID] = append([]byte(nil), data...)
}

func (s *Service) Get(transferID string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.blobs[transferID]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), data...), nil
}

func (s *Service) Exists(transferID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.blobs[transferID]
	return ok
}

func (s *Service) objectKey(transferID string) string {
	if s.prefix == "" {
		return transferID
	}
	return s.prefix + "/" + transferID
}

func (s *Service) signedURL(action, transferID, path string) string {
	expires := time.Now().Add(s.ttl).Unix()
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return ""
	}
	u.Path = path
	query := u.Query()
	query.Set("exp", strconv.FormatInt(expires, 10))
	query.Set("sig", s.signature(action, transferID, expires))
	u.RawQuery = query.Encode()
	return u.String()
}

func (s *Service) signature(action, transferID string, expires int64) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = fmt.Fprintf(mac, "%s\n%s\n%d", action, transferID, expires)
	return hex.EncodeToString(mac.Sum(nil))
}
