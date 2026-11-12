package knowledge

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"site-assistant/internal/core"
)

type FileStorage interface {
	Put(context.Context, []byte) (string, error)
	Read(context.Context, string, int64) ([]byte, error)
	Delete(context.Context, string) error
}
type LocalStorage struct{ Root string }

var safeKey = regexp.MustCompile(`^[a-f0-9-]{36}$`)

func (s LocalStorage) Put(ctx context.Context, b []byte) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if e := os.MkdirAll(s.Root, 0700); e != nil {
		return "", e
	}
	key := core.ID()
	f, e := os.OpenFile(filepath.Join(s.Root, key), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	_, e = f.Write(b)
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		os.Remove(filepath.Join(s.Root, key))
	}
	return key, e
}
func (s LocalStorage) Read(ctx context.Context, key string, max int64) ([]byte, error) {
	if !safeKey.MatchString(key) {
		return nil, core.Fail("invalid_file", 422)
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
