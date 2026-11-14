package objectstorage

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"site-assistant/internal/core"
	"strings"
	"testing"
	"time"
)

func smallPDF() []byte {
	stream := "BT /F1 12 Tf 72 720 Td (Synthetic S3 PDF fixture) Tj ET"
	objects := []string{"<</Type/Catalog/Pages 2 0 R>>", "<</Type/Pages/Kids[4 0 R]/Count 1>>", "<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>", "<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Resources<</Font<</F1 3 0 R>>>>/Contents 5 0 R>>", fmt.Sprintf("<</Length %d>>\nstream\n%s\nendstream", len(stream), stream)}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, o := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer<</Size %d/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF", len(offsets), xref)
	return b.Bytes()
}

// Explicit final cleanup for the NEW synthetic application test DB only.
// No bucket listing or prefix deletion is used: every exact key comes from this DB.
func TestCleanupSyntheticApplicationObjects(t *testing.T) {
	if os.Getenv("S3_APP_TEST_CLEANUP") != "1" {
		t.Skip("explicit final app-test cleanup opt-in")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	u, e := url.Parse(dsn)
	if e != nil || u.Path != "/site_assistant_s3_test" {
		t.Fatal("refusing non-owned application test database")
	}
	c, e := Load()
	if e != nil || c.Provider != "s3" {
		t.Fatal("test S3 config required")
	}
	s := NewS3(c)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal("test DB unavailable")
	}
	defer db.Close()
	rows, e := db.Query(ctx, "SELECT id,organization_id,object_key,identity FROM object_files WHERE provider='s3'")
	if e != nil {
		t.Fatal("test registry unavailable")
	}
	type obj struct{ id, org, key, identity string }
	var objects []obj
	for rows.Next() {
		var o obj
		if rows.Scan(&o.id, &o.org, &o.key, &o.identity) != nil {
			t.Fatal("invalid registry")
		}
		if !strings.HasPrefix(o.key, "integration-tests/site-assistant-stage9a1-") || o.identity != s.Identity {
			t.Fatal("refusing unrelated object")
		}
		objects = append(objects, o)
	}
	if rows.Err() != nil {
		rows.Close()
		t.Fatal("cleanup registry scan failed")
	}
	rows.Close()
	for _, o := range objects {
		if s.Delete(ctx, o.key) != nil {
			t.Fatal("created object cleanup failed")
		}
		if _, _, e = s.Stat(ctx, o.key); e != ErrNotFound {
			t.Fatal("created object still present")
		}
		if _, e = db.Exec(ctx, "UPDATE object_files SET state='deleted' WHERE id=$1 AND organization_id=$2", o.id, o.org); e != nil {
			t.Fatal("cleanup record failed")
		}
	}
	t.Logf("PASS verified absence of %d exact synthetic application objects; no unrelated objects touched", len(objects))
}

func TestS3FailureContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>secret-value</Message></Error>`)
	}))
	defer server.Close()
	s := NewS3(Config{Endpoint: server.URL, Region: "test", Bucket: "test", AccessKey: "invalid-access", SecretKey: "secret-value", PathStyle: true})
	if s.Check(context.Background()) != ErrUnavailable {
		t.Fatal("denied/missing bucket check")
	}
	if e := s.Put(context.Background(), "safe/key", []byte("synthetic"), "text/plain"); e != ErrUnavailable {
		t.Fatal("unsafe error contract")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.Read(ctx, "safe/key", 10); e != ErrUnavailable {
		t.Fatal("canceled request")
	}
	deadline, done := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer done()
	if _, e := s.Read(deadline, "safe/key", 10); e != ErrUnavailable {
		t.Fatal("timeout contract")
	}
	dropped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, e := w.(http.Hijacker).Hijack()
		if e == nil {
			conn.Close()
		}
	}))
	defer dropped.Close()
	broken := NewS3(Config{Endpoint: dropped.URL, Region: "test", Bucket: "test", AccessKey: "test", SecretKey: "test", PathStyle: true})
	if _, e := broken.Read(context.Background(), "safe/key", 10); e != ErrUnavailable {
		t.Fatal("connection loss contract")
	}
	for _, k := range []string{"../x", "/absolute", "a\\b", "a/../b", "a\nx"} {
		if ValidKey(k) {
			t.Fatal("unsafe key")
		}
	}
}

func TestStorageConfigValidation(t *testing.T) {
	t.Setenv("STORAGE_PROVIDER", "local")
	if c, e := Load(); e != nil || c.Provider != "local" {
		t.Fatal("local default")
	}
	for k, v := range map[string]string{"STORAGE_PROVIDER": "s3", "S3_ENDPOINT": "https://storage.yandexcloud.net", "S3_REGION": "ru-central1", "S3_BUCKET": "test-bucket", "S3_ACCESS_KEY": "synthetic", "S3_SECRET_KEY": "synthetic-secret", "S3_KEY_PREFIX": "", "S3_FORCE_PATH_STYLE": "true", "S3_PRESIGN_TTL_SECONDS": "300"} {
		t.Setenv(k, v)
	}
	if _, e := Load(); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ k, v string }{{"S3_ENDPOINT", "http://localhost"}, {"S3_BUCKET", "bad/bucket"}, {"S3_SECRET_KEY", ""}, {"S3_FORCE_PATH_STYLE", "invalid"}, {"S3_PRESIGN_TTL_SECONDS", "90000"}, {"S3_KEY_PREFIX", "../unsafe/"}} {
		t.Run(tc.k, func(t *testing.T) {
			t.Setenv(tc.k, tc.v)
			if _, e := Load(); e == nil {
				t.Fatal("unsafe config accepted")
			}
		})
	}
}
func TestLiveS3(t *testing.T) {
	if os.Getenv("S3_LIVE_TESTS") != "1" {
		t.Skip("explicit S3_LIVE_TESTS=1 required")
	}
	c, e := Load()
	if e != nil {
		t.Fatal(e)
	}
	if c.Provider != "s3" {
		t.Fatal("live S3 configuration required")
	}
	s := NewS3(c)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if e = s.Check(ctx); e != nil {
		t.Fatal("live HeadBucket failed")
	}
	prefix := "integration-tests/" + core.ID() + "/"
	var pngBytes bytes.Buffer
	png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	pdf := smallPDF()
	fixtures := []struct {
		name, typ string
		b         []byte
	}{{"text", "text/plain", []byte("Synthetic RU Русский EN English CS Čeština DE Deutsch")}, {"pdf", "application/pdf", pdf}, {"png", "image/png", pngBytes.Bytes()}}
	for _, f := range fixtures {
		k := prefix + f.name
		t.Cleanup(func() {
			clean, done := context.WithTimeout(context.Background(), 20*time.Second)
			defer done()
			if s.Delete(clean, k) != nil {
				t.Error("test object cleanup failed")
			}
			if _, _, e := s.Stat(clean, k); e != ErrNotFound {
				t.Error("test object deletion not confirmed")
			}
		})
		if e = s.Put(ctx, k, f.b, f.typ); e != nil {
			t.Fatal("live upload failed")
		}
		n, typ, e := s.Stat(ctx, k)
		if e != nil || n != int64(len(f.b)) || typ != f.typ {
			t.Fatal("live HEAD mismatch")
		}
		if f.name == "text" {
			badAccess := c
			badAccess.AccessKey = "synthetic-invalid-access-key"
			if _, _, e := NewS3(badAccess).Stat(ctx, k); e != ErrUnavailable {
				t.Fatal("invalid access key was not rejected")
			}
			badSecret := c
			badSecret.SecretKey = "synthetic-invalid-secret-key"
			if _, _, e := NewS3(badSecret).Stat(ctx, k); e != ErrUnavailable {
				t.Fatal("invalid secret key was not rejected")
			}
			t.Log("live invalid access/secret key HEAD requests rejected; configured key unchanged")
		}
		b, e := s.Read(ctx, k, 1<<20)
		if e != nil || Hash(b) != Hash(f.b) {
			t.Fatal("live SHA256 mismatch")
		}
		t.Logf("verified %s bytes=%d sha256=%s", f.name, len(b), Hash(b))
		u := c.Endpoint + "/" + c.Bucket + "/" + k
		client := &http.Client{Timeout: 5 * time.Second}
		r, e := client.Get(u)
		if e != nil {
			t.Fatal("public access probe failed")
		}
		io.Copy(io.Discard, io.LimitReader(r.Body, 1024))
		r.Body.Close()
		if r.StatusCode != 403 && r.StatusCode != 401 {
			t.Fatal("unauthorized read was not denied")
		}
	}
	if _, _, e = s.Stat(ctx, prefix+"missing"); e != ErrNotFound {
		t.Fatal("missing object behavior")
	}
	t.Log("live synthetic objects verified; cleanup checks run before test completes")
}
