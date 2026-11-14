package objectstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrUnavailable = errors.New("object storage unavailable")
var ErrNotFound = errors.New("object not found")
var ErrIntegrity = errors.New("object integrity check failed")

type Config struct {
	Provider, Endpoint, Region, Bucket, AccessKey, SecretKey, Prefix string
	PathStyle                                                        bool
}

func Load() (Config, error) {
	c := Config{Provider: os.Getenv("STORAGE_PROVIDER"), Endpoint: os.Getenv("S3_ENDPOINT"), Region: os.Getenv("S3_REGION"), Bucket: os.Getenv("S3_BUCKET"), AccessKey: os.Getenv("S3_ACCESS_KEY"), SecretKey: os.Getenv("S3_SECRET_KEY"), Prefix: os.Getenv("S3_KEY_PREFIX"), PathStyle: true}
	if c.Provider == "" {
		c.Provider = "local"
	}
	if c.Provider != "local" && c.Provider != "s3" {
		return c, errors.New("invalid STORAGE_PROVIDER")
	}
	if c.Provider == "local" {
		c.Endpoint = strings.TrimRight(c.Endpoint, "/")
		if v, e := strconv.ParseBool(os.Getenv("S3_FORCE_PATH_STYLE")); e == nil {
			c.PathStyle = v
		}
		return c, nil
	}
	u, e := url.Parse(c.Endpoint)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return c, errors.New("invalid S3_ENDPOINT")
	}
	if c.Region == "" || c.Bucket == "" || c.AccessKey == "" || c.SecretKey == "" || strings.ContainsAny(c.Bucket, "/\\ \r\n") {
		return c, errors.New("S3 configuration required")
	}
	c.Endpoint = strings.TrimRight(c.Endpoint, "/")
	if v := os.Getenv("S3_FORCE_PATH_STYLE"); v != "" {
		c.PathStyle, e = strconv.ParseBool(v)
		if e != nil {
			return c, errors.New("invalid S3_FORCE_PATH_STYLE")
		}
	}
	if v := os.Getenv("S3_PRESIGN_TTL_SECONDS"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 60 || n > 900 {
			return c, errors.New("invalid S3_PRESIGN_TTL_SECONDS")
		}
	}
	if c.Prefix != "" && !regexp.MustCompile(`^integration-tests/[a-zA-Z0-9-]+/$`).MatchString(c.Prefix) {
		return c, errors.New("S3_KEY_PREFIX must be an isolated integration test prefix")
	}
	if c.Prefix != "" && os.Getenv("APP_ENV") == "production" {
		return c, errors.New("test prefix forbidden in production")
	}
	return c, nil
}

type S3 struct {
	Client           *s3.Client
	Bucket, Identity string
	checkMu          sync.Mutex
	checkedAt        time.Time
	checkError       error
}

// Local mode may ignore unused S3 settings. Only a fully configured HTTPS
// client can be retained for reading objects migrated in an earlier deployment.
func ConfiguredS3(c Config) bool {
	u, e := url.Parse(c.Endpoint)
	return e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && (u.Path == "" || u.Path == "/") && c.Region != "" && c.Bucket != "" && !strings.ContainsAny(c.Bucket, "/\\ \r\n") && c.AccessKey != "" && c.SecretKey != ""
}

func NewS3(c Config) *S3 {
	h := sha256.Sum256([]byte(c.Endpoint + "\n" + c.Region + "\n" + c.Bucket))
	return &S3{Client: s3.New(s3.Options{Region: c.Region, BaseEndpoint: aws.String(c.Endpoint), UsePathStyle: c.PathStyle, Credentials: credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, ""), HTTPClient: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, RetryMaxAttempts: 2, RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired}), Bucket: c.Bucket, Identity: hex.EncodeToString(h[:])}
}
func safeError(e error) error {
	if e == nil {
		return nil
	}
	var api smithy.APIError
	if errors.As(e, &api) && (api.ErrorCode() == "NoSuchKey" || api.ErrorCode() == "NotFound") {
		return ErrNotFound
	}
	return ErrUnavailable
}
func ValidKey(k string) bool {
	return k != "" && len(k) < 512 && !strings.HasPrefix(k, "/") && !strings.Contains(k, "..") && !strings.ContainsAny(k, "\\\r\n\x00") && regexp.MustCompile(`^[a-zA-Z0-9/_-]+$`).MatchString(k)
}
func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (s *S3) Check(ctx context.Context) error {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	if ctx.Err() != nil {
		return ErrUnavailable
	}
	if time.Since(s.checkedAt) < 5*time.Second {
		return s.checkError
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, e := s.Client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.Bucket})
	s.checkedAt = time.Now()
	s.checkError = safeError(e)
	return s.checkError
}
func (s *S3) Put(ctx context.Context, k string, b []byte, typ string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !ValidKey(k) || len(b) > 50<<20 {
		return ErrUnavailable
	}
	_, e := s.Client.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.Bucket, Key: &k, Body: bytes.NewReader(b), ContentLength: aws.Int64(int64(len(b))), ContentType: &typ, Metadata: map[string]string{"sha256": Hash(b)}})
	return safeError(e)
}
func (s *S3) Stat(ctx context.Context, k string) (int64, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !ValidKey(k) {
		return 0, "", ErrUnavailable
	}
	o, e := s.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.Bucket, Key: &k})
	if e != nil {
		return 0, "", safeError(e)
	}
	return aws.ToInt64(o.ContentLength), aws.ToString(o.ContentType), nil
}
func (s *S3) Read(ctx context.Context, k string, max int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !ValidKey(k) || max < 0 || max > 50<<20 {
		return nil, ErrUnavailable
	}
	o, e := s.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.Bucket, Key: &k})
	if e != nil {
		return nil, safeError(e)
	}
	defer o.Body.Close()
	if aws.ToInt64(o.ContentLength) > max {
		return nil, ErrUnavailable
	}
	b, e := io.ReadAll(io.LimitReader(o.Body, max+1))
	if e != nil || int64(len(b)) > max {
		return nil, ErrUnavailable
	}
	return b, nil
}
func (s *S3) Delete(ctx context.Context, k string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !ValidKey(k) {
		return ErrUnavailable
	}
	_, e := s.Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.Bucket, Key: &k})
	return safeError(e)
}
