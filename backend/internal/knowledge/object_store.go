package knowledge

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"site-assistant/internal/core"
	"site-assistant/internal/objectstorage"
)

// ObjectStore preserves opaque UUID references while resolving physical storage on the server.
type ObjectStore struct {
	DB                    *pgxpool.Pool
	Local                 LocalStorage
	S3                    *objectstorage.S3
	Config                objectstorage.Config
	Purpose, Organization string
}

func NewObjectStore(db *pgxpool.Pool, c objectstorage.Config, root, purpose string) *ObjectStore {
	v := &ObjectStore{DB: db, Local: LocalStorage{Root: root}, Config: c, Purpose: purpose}
	if c.Provider == "s3" || objectstorage.ConfiguredS3(c) {
		v.S3 = objectstorage.NewS3(c)
	}
	return v
}
func Scoped(f FileStorage, org string) FileStorage {
	if s, ok := f.(*ObjectStore); ok {
		c := *s
		c.Organization = org
		return &c
	}
	return f
}
func CheckStorage(ctx context.Context, f FileStorage, root string) error {
	if s, ok := f.(*ObjectStore); ok {
		return s.Check(ctx)
	}
	return checkLocal(root)
}
func checkLocal(root string) error { return (LocalStorage{Root: root}).Check(context.Background()) }
func (s *ObjectStore) Check(ctx context.Context) error {
	var existing bool
	if e := s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM object_files WHERE purpose=$1 AND provider='s3' AND state<>'deleted')", s.Purpose).Scan(&existing); e != nil {
		return objectstorage.ErrUnavailable
	}
	if s.Config.Provider == "s3" || existing {
		if s.S3 == nil {
			return objectstorage.ErrUnavailable
		}
		var mismatch bool
		if e := s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM object_files WHERE purpose=$1 AND provider='s3' AND state<>'deleted' AND identity<>$2)", s.Purpose, s.S3.Identity).Scan(&mismatch); e != nil || mismatch {
			return objectstorage.ErrUnavailable
		}
		return s.S3.Check(ctx)
	}
	return s.Local.Check(ctx)
}

type stored struct {
	Org, Provider, Key, Identity, Hash, State string
	Bytes                                     *int64
}

func (s *ObjectStore) lookup(ctx context.Context, id string) (stored, error) {
	return s.lookupIn(ctx, s.DB, id)
}

type objectQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *ObjectStore) lookupIn(ctx context.Context, q objectQuery, id string) (stored, error) {
	v := stored{}
	if !safeKey.MatchString(id) {
		return v, objectstorage.ErrUnavailable
	}
	e := q.QueryRow(ctx, "SELECT organization_id,provider,object_key,identity,sha256,state,bytes FROM object_files WHERE id=$1 AND purpose=$2 AND ($3='' OR organization_id=NULLIF($3,'')::uuid)", id, s.Purpose, s.Organization).Scan(&v.Org, &v.Provider, &v.Key, &v.Identity, &v.Hash, &v.State, &v.Bytes)
	if e != nil {
		if e == pgx.ErrNoRows {
			return v, objectstorage.ErrNotFound
		}
		return v, objectstorage.ErrUnavailable
	}
	if s.Organization != "" && s.Organization != v.Org {
		return v, objectstorage.ErrNotFound
	}
	if v.Provider == "s3" && (s.S3 == nil || v.Identity != s.S3.Identity) {
		return v, objectstorage.ErrUnavailable
	}
	return v, nil
}
func (s *ObjectStore) Put(ctx context.Context, b []byte) (string, error) {
	id := core.ID()
	return id, s.PutAt(ctx, id, b)
}
func (s *ObjectStore) PutAt(ctx context.Context, id string, b []byte) error {
	if s.Organization == "" || !safeKey.MatchString(id) || len(b) > 50<<20 {
		return objectstorage.ErrUnavailable
	}
	key := id
	identity := ""
	provider := s.Config.Provider
	if provider == "s3" {
		if s.S3 == nil {
			return objectstorage.ErrUnavailable
		}
		key = s.Config.Prefix + "organizations/" + s.Organization + "/" + s.Purpose + "/" + id
		identity = s.S3.Identity
	}
	_, e := s.DB.Exec(ctx, "INSERT INTO object_files(id,organization_id,purpose,provider,object_key,identity,bytes,sha256,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending')", id, s.Organization, s.Purpose, provider, key, identity, len(b), objectstorage.Hash(b))
	if e != nil {
		return objectstorage.ErrUnavailable
	}
	if provider == "s3" {
		e = s.S3.Put(ctx, key, b, http.DetectContentType(b))
	} else {
		e = s.Local.PutAt(ctx, key, b)
	}
	if e != nil {
		return e
	}
	var read []byte
	if provider == "s3" {
		read, e = s.S3.Read(ctx, key, int64(len(b)))
	} else {
		read, e = s.Local.Read(ctx, key, int64(len(b)))
	}
	if e != nil {
		return e
	}
	if objectstorage.Hash(read) != objectstorage.Hash(b) {
		return objectstorage.ErrIntegrity
	}
	_, e = s.DB.Exec(ctx, "UPDATE object_files SET state='ready' WHERE id=$1 AND organization_id=$2 AND state='pending'", id, s.Organization)
	if e != nil {
		return objectstorage.ErrUnavailable
	}
	return nil
}
func (s *ObjectStore) Read(ctx context.Context, id string, max int64) ([]byte, error) {
	v, e := s.lookup(ctx, id)
	if e != nil {
		return nil, e
	}
	if v.State != "ready" {
		return nil, objectstorage.ErrUnavailable
	}
	var b []byte
	if v.Provider == "s3" {
		b, e = s.S3.Read(ctx, v.Key, max)
	} else {
		b, e = s.Local.Read(ctx, v.Key, max)
	}
	if e != nil {
		return nil, objectstorage.ErrUnavailable
	}
	if v.Hash != "" && objectstorage.Hash(b) != v.Hash {
		return nil, objectstorage.ErrIntegrity
	}
	if v.Bytes != nil && int64(len(b)) != *v.Bytes {
		return nil, objectstorage.ErrIntegrity
	}
	return b, nil
}
func (s *ObjectStore) Size(ctx context.Context, id string) (int64, error) {
	v, e := s.lookup(ctx, id)
	if e != nil {
		return 0, e
	}
	if v.State != "ready" {
		return 0, objectstorage.ErrNotFound
	}
	if v.Provider == "s3" {
		n, _, e := s.S3.Stat(ctx, v.Key)
		if e == nil && v.Bytes != nil && n != *v.Bytes {
			return 0, objectstorage.ErrIntegrity
		}
		return n, e
	}
	n, e := s.Local.Size(ctx, v.Key)
	if e == nil && v.Bytes != nil && n != *v.Bytes {
		return 0, objectstorage.ErrIntegrity
	}
	return n, e
}
func (s *ObjectStore) Delete(ctx context.Context, id string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return objectstorage.ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var locked string
	e = tx.QueryRow(ctx, "SELECT id FROM object_files WHERE id=$1 AND purpose=$2 AND ($3='' OR organization_id=NULLIF($3,'')::uuid) FOR UPDATE", id, s.Purpose, s.Organization).Scan(&locked)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return objectstorage.ErrUnavailable
	}
	v, e := s.lookupIn(ctx, tx, id)
	if e != nil {
		return e
	}
	if v.State == "deleted" {
		return nil
	}
	if v.Provider == "s3" {
		e = s.S3.Delete(ctx, v.Key)
	} else {
		e = s.Local.Delete(ctx, v.Key)
	}
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "UPDATE object_files SET state='deleted' WHERE id=$1 AND organization_id=$2", id, v.Org)
	if e != nil {
		return objectstorage.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return objectstorage.ErrUnavailable
	}
	return nil
}

type MigrationResult struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// MigrateOne is repeatable, never removes local originals and never alters billing counters.
func (s *ObjectStore) MigrateOne(ctx context.Context, id string, apply bool) (MigrationResult, error) {
	r := MigrationResult{ID: id, State: "missing"}
	v, e := s.lookup(ctx, id)
	if e != nil {
		return r, e
	}
	if v.Provider == "s3" {
		_, e = s.Read(ctx, id, 50<<20)
		r.State = "verified_s3"
		return r, e
	}
	b, e := s.Local.Read(ctx, v.Key, 50<<20)
	if e != nil {
		return r, nil
	}
	if v.Hash != "" && objectstorage.Hash(b) != v.Hash {
		return r, objectstorage.ErrIntegrity
	}
	r.State = "would_copy"
	if !apply {
		return r, nil
	}
	if s.S3 == nil {
		return r, objectstorage.ErrUnavailable
	}
	// Register an isolated pending copy BEFORE network I/O. On rollback it stays
	// discoverable by orphan cleanup; an uncertain commit must never delete it.
	copyStore := *s
	copyStore.Organization = v.Org
	copyStore.Config.Provider = "s3"
	copyID := core.ID()
	if e = copyStore.PutAt(ctx, copyID, b); e != nil {
		return r, e
	}
	copied, e := copyStore.lookup(ctx, copyID)
	if e != nil {
		return r, e
	}
	key := copied.Key
	// Only the final metadata switch holds a database transaction.
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return r, e
	}
	defer tx.Rollback(ctx)
	var provider string
	e = tx.QueryRow(ctx, "SELECT provider FROM object_files WHERE id=$1 AND organization_id=$2 AND state='ready' FOR UPDATE", id, v.Org).Scan(&provider)
	if e != nil {
		return r, e
	}
	if provider != "local" {
		return r, fmt.Errorf("migration changed; retry")
	}
	if _, e = tx.Exec(ctx, "DELETE FROM object_files WHERE id=$1 AND organization_id=$2 AND state='ready'", copyID, v.Org); e != nil {
		return r, e
	}
	_, e = tx.Exec(ctx, "UPDATE object_files SET provider='s3',object_key=$3,identity=$4,bytes=$5,sha256=$6 WHERE id=$1 AND organization_id=$2 AND provider='local'", id, v.Org, key, s.S3.Identity, len(b), objectstorage.Hash(b))
	if e != nil {
		return r, e
	}
	r.State = "copied_verified_local_retained"
	return r, tx.Commit(ctx)
}
func (s *ObjectStore) Candidates(ctx context.Context, after string) ([]string, error) {
	rows, e := s.DB.Query(ctx, "SELECT id FROM object_files f WHERE purpose=$1 AND state='ready' AND ($2='' OR id>NULLIF($2,'')::uuid) AND (EXISTS(SELECT 1 FROM knowledge_sources k WHERE k.organization_id=f.organization_id AND k.file_key=f.id::text) OR EXISTS(SELECT 1 FROM widgets w WHERE w.organization_id=f.organization_id AND w.logo_key=f.id::text)) ORDER BY id LIMIT 100", s.Purpose, after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

var _ FileStorage = (*ObjectStore)(nil)

func (s *ObjectStore) Orphans(ctx context.Context) ([]string, error) {
	rows, e := s.DB.Query(ctx, "SELECT f.id FROM object_files f WHERE purpose=$1 AND state IN ('pending','ready') AND created_at<now()-interval '1 day' AND NOT EXISTS(SELECT 1 FROM knowledge_sources k WHERE k.organization_id=f.organization_id AND k.file_key=f.id::text) AND NOT EXISTS(SELECT 1 FROM widgets w WHERE w.organization_id=f.organization_id AND w.logo_key=f.id::text) ORDER BY f.id LIMIT 100", s.Purpose)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Export/restore operate on explicit known IDs, never enumerate the remote bucket.
func (s *ObjectStore) ExportOne(ctx context.Context, id, dir string) error {
	b, e := s.Read(ctx, id, 50<<20)
	if e != nil {
		return e
	}
	local := LocalStorage{Root: dir}
	if existing, e := local.Read(ctx, id, 50<<20); e == nil {
		if checksum(existing) != checksum(b) {
			return objectstorage.ErrIntegrity
		}
		return nil
	}
	return local.PutAt(ctx, id, b)
}
func (s *ObjectStore) RestoreLocalOne(ctx context.Context, id, dir string, apply bool) error {
	// Recovery verifies the offline export against DB metadata without contacting
	// the old bucket or requiring credentials that may have been lost/revoked.
	v := stored{}
	e := s.DB.QueryRow(ctx, "SELECT organization_id,sha256,bytes FROM object_files WHERE id=$1 AND purpose=$2 AND state='ready' AND ($3='' OR organization_id=NULLIF($3,'')::uuid)", id, s.Purpose, s.Organization).Scan(&v.Org, &v.Hash, &v.Bytes)
	if e != nil {
		return e
	}
	b, e := (LocalStorage{Root: dir}).Read(ctx, id, 50<<20)
	if e != nil {
		return objectstorage.ErrUnavailable
	}
	if v.Hash != "" && v.Hash != checksum(b) {
		return objectstorage.ErrIntegrity
	}
	if v.Bytes != nil && *v.Bytes != int64(len(b)) {
		return objectstorage.ErrIntegrity
	}
	if !apply {
		return nil
	}
	_, e = s.DB.Exec(ctx, "UPDATE object_files SET provider='local',identity='',object_key=id::text,bytes=$3,sha256=$4 WHERE id=$1 AND organization_id=$2 AND state='ready'", id, v.Org, len(b), checksum(b))
	return e
}
