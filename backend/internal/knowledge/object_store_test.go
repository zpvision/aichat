package knowledge

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"site-assistant/internal/core"
	"site-assistant/internal/objectstorage"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func cleanupLiveRegistry(t *testing.T, r *ObjectStore, prefix string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		rows, e := r.DB.Query(ctx, "SELECT object_key FROM object_files WHERE provider='s3' AND identity=$1", r.S3.Identity)
		if e != nil {
			t.Error("cleanup registry unavailable")
			return
		}
		var keys []string
		for rows.Next() {
			var key string
			if rows.Scan(&key) != nil {
				t.Error("cleanup registry invalid")
				continue
			}
			if !strings.HasPrefix(key, prefix) {
				t.Error("refusing non-test key")
				continue
			}
			keys = append(keys, key)
		}
		if rows.Err() != nil {
			rows.Close()
			t.Error("cleanup registry scan failed")
			return
		}
		rows.Close()
		for _, key := range keys {
			if r.S3.Delete(ctx, key) != nil {
				t.Error("test object cleanup failed")
			}
			if _, _, e := r.S3.Stat(ctx, key); e != objectstorage.ErrNotFound {
				t.Error("test object remains")
			}
		}
	})
}

func TestObjectStorageFailureAndQuotaRollback(t *testing.T) {
	s, user, org, a := setup(t)
	var mu sync.Mutex
	objects := map[string][]byte{}
	denyPut, denyRead, corrupt := false, false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case "PUT":
			if denyPut {
				w.WriteHeader(403)
				return
			}
			objects[r.URL.Path], _ = io.ReadAll(r.Body)
		case "DELETE":
			delete(objects, r.URL.Path)
			w.WriteHeader(204)
		case "GET":
			if denyRead {
				w.WriteHeader(403)
				return
			}
			b, ok := objects[r.URL.Path]
			if !ok {
				w.WriteHeader(404)
				return
			}
			if corrupt {
				b = []byte("tampered")
			}
			w.Write(b)
		case "HEAD":
			if _, ok := objects[r.URL.Path]; !ok {
				w.WriteHeader(404)
			}
		}
	}))
	defer server.Close()
	c := objectstorage.Config{Provider: "s3", Endpoint: server.URL, Region: "test", Bucket: "test", AccessKey: "test", SecretKey: "test", PathStyle: true}
	r := NewObjectStore(s.Store.DB, c, s.Config.FileRoot, "knowledge")
	s.Files = r
	ctx := context.Background()
	for _, mode := range []string{"write", "read", "corrupt", "database"} {
		mu.Lock()
		denyPut = mode == "write"
		denyRead = mode == "read"
		corrupt = mode == "corrupt"
		mu.Unlock()
		if mode == "database" {
			if _, e := s.Store.DB.Exec(ctx, "CREATE FUNCTION fail_object_ready() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='ready' THEN RAISE EXCEPTION 'simulated failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_ready BEFORE UPDATE ON object_files FOR EACH ROW EXECUTE FUNCTION fail_object_ready()"); e != nil {
				t.Fatal(e)
			}
		}
		if _, e := s.Upload(ctx, user, org, a, "synthetic.txt", []byte("Synthetic upload rollback fixture")); e == nil {
			t.Fatal("failure accepted", mode)
		}
		mu.Lock()
		n := len(objects)
		mu.Unlock()
		if n != 0 {
			t.Fatal("unused S3 object left", mode, n)
		}
		var used, reserved int64
		if e := s.Store.DB.QueryRow(ctx, "SELECT (resources->>'knowledge_bytes')::bigint,(resources->>'reserved_bytes')::bigint FROM billing_subscriptions WHERE organization_id=$1", org).Scan(&used, &reserved); e != nil || used != 0 || reserved != 0 {
			t.Fatal("quota leaked", used, reserved, e)
		}
	}
	if _, e := s.Store.DB.Exec(ctx, "DROP TRIGGER fail_ready ON object_files; UPDATE billing_periods SET limits=jsonb_set(limits,'{knowledge_bytes}','10'::jsonb)"); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	denyPut = false
	denyRead = false
	corrupt = false
	mu.Unlock()
	var wg sync.WaitGroup
	var won atomic.Int32
	var idsMu sync.Mutex
	var ids []string
	for _, b := range []string{"abcdef", "ghijkl"} {
		wg.Add(1)
		go func(b string) {
			defer wg.Done()
			v, e := s.Upload(ctx, user, org, a, "quota.txt", []byte(b))
			if e == nil {
				won.Add(1)
				idsMu.Lock()
				ids = append(ids, v.ID)
				idsMu.Unlock()
			}
		}(b)
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatal("concurrent S3 quota admission", won.Load())
	}
	for _, id := range ids {
		if e := s.Delete(ctx, user, org, a, id); e != nil {
			t.Fatal(e)
		}
	}
}

func TestObjectRegistryLocal(t *testing.T) {
	s, user, org, a := setup(t)
	r := NewObjectStore(s.Store.DB, objectstorage.Config{Provider: "local"}, s.Config.FileRoot, "knowledge")
	s.Files = r
	v, e := s.Upload(context.Background(), user, org, a, "test.txt", []byte("Synthetic registry document"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Scoped(r, core.ID()).Read(context.Background(), v.ID, 100); e == nil {
		t.Fatal("tenant scope bypass")
	}
	if _, e = s.Original(context.Background(), user, org, a, v.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.Exec(context.Background(), "UPDATE object_files SET provider='s3',identity='unreachable-old-bucket',object_key='unreachable/test' WHERE id=$1 AND organization_id=$2", v.ID, org); e != nil {
		t.Fatal(e)
	}
	if e = r.RestoreLocalOne(context.Background(), v.ID, s.Config.FileRoot, false); e != nil {
		t.Fatal("offline restore dry run", e)
	}
	if e = r.RestoreLocalOne(context.Background(), v.ID, s.Config.FileRoot, true); e != nil {
		t.Fatal("restore without S3 credentials", e)
	}
	if _, e = s.Original(context.Background(), user, org, a, v.ID); e != nil {
		t.Fatal("restored local original", e)
	}
	if e = s.Delete(context.Background(), user, org, a, v.ID); e != nil {
		t.Fatal(e)
	}
	if e = r.Delete(context.Background(), v.ID); e != nil {
		t.Fatal("idempotent delete", e)
	}
}

func TestLiveKnowledgeS3(t *testing.T) {
	if os.Getenv("S3_LIVE_TESTS") != "1" {
		t.Skip("explicit live opt-in")
	}
	s, user, org, a := setup(t)
	c, e := objectstorage.Load()
	if e != nil || c.Provider != "s3" {
		t.Fatal("S3 config required")
	}
	c.Prefix = "integration-tests/" + core.ID() + "/"
	r := NewObjectStore(s.Store.DB, c, s.Config.FileRoot, "knowledge")
	cleanupLiveRegistry(t, r, c.Prefix)
	s.Files = r
	s.Provider = &Ollama{BaseURL: s.Config.BaseURL, Model: s.Config.Model, Client: embeddingClient(s.Config)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	id := ""
	t.Cleanup(func() {
		if id != "" {
			clean, done := context.WithTimeout(context.Background(), 20*time.Second)
			defer done()
			if r.Delete(clean, id) != nil {
				t.Error("cleanup failed")
			}
			if _, _, e := r.S3.Stat(clean, c.Prefix+"organizations/"+org+"/knowledge/"+id); e != objectstorage.ErrNotFound {
				t.Error("cleanup not confirmed")
			}
		}
	})
	b := pdfFixture([]string{"Synthetic Aurora accounting support costs 450 EUR monthly."})
	v, e := s.Upload(ctx, user, org, a, "synthetic.pdf", b)
	id = v.ID
	if e != nil {
		t.Fatal("S3 PDF upload failed")
	}
	claimed, e := s.Store.Claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.Exec(ctx, "UPDATE knowledge_jobs SET lease_until=now()-interval '1 second' WHERE id=$1", claimed.ID); e != nil {
		t.Fatal(e)
	}
	// Recreate the service with an EMPTY local directory after an abandoned lease.
	restarted := New(s.Auth, s.Config)
	restarted.Files = NewObjectStore(s.Store.DB, c, t.TempDir(), "knowledge")
	restarted.Provider = s.Provider
	s = restarted
	if e = s.ProcessOne(ctx); e != nil {
		t.Fatal("real PDF/embedding processing", e)
	}
	v, e = s.Get(ctx, user, org, a, v.ID)
	if e != nil || v.Status != "ready" {
		t.Fatal("index not ready")
	}
	hits, e := s.Search(ctx, user, org, a, SearchInput{Query: "Aurora accounting cost", Mode: "hybrid"})
	if e != nil || len(hits) == 0 || hits[0].SourceID != id || hits[0].Model != "bge-m3" {
		t.Fatal("real RAG failed", e)
	}
	download, e := s.Original(ctx, user, org, a, id)
	if e != nil || checksum(download) != checksum(b) {
		t.Fatal("original integrity")
	}
	if _, e = s.Original(ctx, core.ID(), org, a, id); e == nil {
		t.Fatal("foreign access")
	}
	old := *v.ActiveRevision
	if _, e = s.Reindex(ctx, user, org, a, id); e != nil {
		t.Fatal(e)
	}
	v, _ = s.Get(ctx, user, org, a, id)
	if *v.ActiveRevision != old {
		t.Fatal("old revision lost")
	}
	if e = s.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Delete(ctx, user, org, a, id); e != nil {
		t.Fatal(e)
	}
	for _, fixture := range []struct {
		name string
		b    []byte
	}{{"synthetic.txt", []byte("Synthetic text knowledge for multilingual accounting support.")}, {"synthetic.md", []byte("# Synthetic knowledge\n\nAccounting support costs 450 EUR monthly.")}, {"synthetic.docx", docx(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Synthetic document accounting support costs 450 EUR monthly.</w:t></w:r></w:p></w:body></w:document>`)}} {
		other, e := s.Upload(ctx, user, org, a, fixture.name, fixture.b)
		if e != nil {
			t.Fatal("live format upload", fixture.name, e)
		}
		if e = s.ProcessOne(ctx); e != nil {
			t.Fatal("live format extraction/embedding", fixture.name, e)
		}
		other, e = s.Get(ctx, user, org, a, other.ID)
		if e != nil || other.Status != "ready" {
			t.Fatal("live format not indexed", fixture.name)
		}
		if e = s.Delete(ctx, user, org, a, other.ID); e != nil {
			t.Fatal(e)
		}
	}
	t.Log("PASS real Yandex S3 PDF -> extraction -> BGE-M3 -> hybrid RAG, original SHA256, tenant isolation, reindex/delete")
}

func TestLiveMigrationS3(t *testing.T) {
	if os.Getenv("S3_LIVE_TESTS") != "1" {
		t.Skip("explicit live opt-in")
	}
	s, user, org, a := setup(t)
	c, e := objectstorage.Load()
	if e != nil {
		t.Fatal(e)
	}
	c.Prefix = "integration-tests/" + core.ID() + "/"
	c.Provider = "local"
	r := NewObjectStore(s.Store.DB, c, s.Config.FileRoot, "knowledge")
	cleanupLiveRegistry(t, r, c.Prefix)
	s.Files = r
	ctx := context.Background()
	v, e := s.Upload(ctx, user, org, a, "migrate.txt", []byte("Synthetic migration source"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if r.Delete(context.Background(), v.ID) != nil {
			t.Error("migration object cleanup failed")
		}
	})
	if result, e := r.MigrateOne(ctx, v.ID, false); e != nil || result.State != "would_copy" {
		t.Fatal(result, e)
	}
	if result, e := r.MigrateOne(ctx, v.ID, true); e != nil || result.State != "copied_verified_local_retained" {
		t.Fatal(result, e)
	}
	if result, e := r.MigrateOne(ctx, v.ID, true); e != nil || result.State != "verified_s3" {
		t.Fatal(result, e)
	}
	if _, e = r.Local.Size(ctx, v.ID); e != nil {
		t.Fatal("local original deleted")
	}
	missing, e := s.Upload(ctx, user, org, a, "missing.txt", []byte("Synthetic missing original"))
	if e != nil {
		t.Fatal(e)
	}
	r.Local.Delete(ctx, missing.ID)
	if result, e := r.MigrateOne(ctx, missing.ID, true); e != nil || result.State != "missing" {
		t.Fatal(result, e)
	}
	if _, e = s.Get(ctx, user, org, a, missing.ID); e != nil {
		t.Fatal("missing original metadata lost")
	}
}
