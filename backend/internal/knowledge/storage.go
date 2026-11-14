package knowledge

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"site-assistant/internal/core"
	"site-assistant/internal/ops"
)

type FileStorage interface {
	Put(context.Context, []byte) (string, error)
	PutAt(context.Context, string, []byte) error
	Size(context.Context, string) (int64, error)
	Read(context.Context, string, int64) ([]byte, error)
	Delete(context.Context, string) error
}
type LocalStorage struct{ Root string }

func (s LocalStorage) Check(ctx context.Context) error { return ops.Writable(s.Root) }

var safeKey = regexp.MustCompile(`^[a-f0-9-]{36}$`)

func (s LocalStorage) Put(ctx context.Context, b []byte) (string, error) {
	key := core.ID()
	return key, s.PutAt(ctx, key, b)
}
func (s LocalStorage) Size(ctx context.Context, key string) (int64, error) {
	if !safeKey.MatchString(key) {
		return 0, core.Fail("invalid_file", 422)
	}
	info, e := os.Lstat(filepath.Join(s.Root, key))
	if e != nil {
		return 0, e
	}
	if !info.Mode().IsRegular() {
		return 0, core.Fail("invalid_file", 422)
	}
	return info.Size(), nil
}
func (s LocalStorage) PutAt(ctx context.Context, key string, b []byte) error {
	if !safeKey.MatchString(key) {
		return core.Fail("invalid_file", 422)
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := os.MkdirAll(s.Root, 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(s.Root, key), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		os.Remove(filepath.Join(s.Root, key))
	}
	return e
}
func (s LocalStorage) Read(ctx context.Context, key string, max int64) ([]byte, error) {
	if !safeKey.MatchString(key) {
		return nil, core.Fail("invalid_file", 422)
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	n, e := s.Size(ctx, key)
	if e != nil {
		return nil, e
	}
	if max < 0 || n > max {
		return nil, core.Fail("file_too_large", 413)
	}
	f, e := os.Open(filepath.Join(s.Root, key))
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, max+1))
	if int64(len(b)) > max {
		return nil, core.Fail("file_too_large", 413)
	}
	return b, e
}
func (s LocalStorage) Delete(ctx context.Context, key string) error {
	if !safeKey.MatchString(key) {
		return core.Fail("invalid_file", 422)
	}
	e := os.Remove(filepath.Join(s.Root, key))
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
