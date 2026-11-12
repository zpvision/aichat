package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/argon2"
	"net/mail"
	"strings"
)

func ID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func HashPassword(password string) string {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		panic(e)
	}
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return "$argon2id$v=19$m=65536,t=3,p=2$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
}
func CheckPassword(hash, password string) bool {
	p := strings.Split(hash, "$")
	if len(p) != 6 || strings.Join(p[:4], "$") != "$argon2id$v=19$m=65536,t=3,p=2" {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(p[4])
	if e != nil || len(salt) != 16 {
		return false
	}
	expected, e := base64.RawStdEncoding.DecodeString(p[5])
	if e != nil || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
func TokenHash(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func (s *Service) NewSession(ctx context.Context, user string) (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, s.Store.AddSession(ctx, TokenHash(token), user)
}
func (s *Service) Register(ctx context.Context, email, password, locale string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	a, e := mail.ParseAddress(email)
	if e != nil || a.Address != email || len(email) > 254 || len(password) < 12 || len(password) > 128 || !validLocale(locale) {
		return User{}, Fail("validation", 422)
	}
	u, e := s.Store.AddUser(ctx, ID(), email, HashPassword(password), locale)
	if e != nil && isUnique(e) {
		return User{}, nil
	}
	return u, e
}
func (s *Service) Login(ctx context.Context, email, password string) (User, error) {
	if len(password) > 128 {
		return User{}, Fail("invalid_credentials", 401)
	}
	id, hash, e := s.Store.Credentials(ctx, strings.ToLower(strings.TrimSpace(email)))
	if e != nil && e != pgx.ErrNoRows {
		return User{}, e
	}
	if e == pgx.ErrNoRows {
		hash = s.dummyHash
	}
	ok := CheckPassword(hash, password)
	if !ok || e != nil {
		return User{}, Fail("invalid_credentials", 401)
	}
	return s.Store.User(ctx, id)
}
