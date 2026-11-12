package core

import "testing"

func TestPassword(t *testing.T) {
	h := HashPassword("a-strong-password")
	if !CheckPassword(h, "a-strong-password") || CheckPassword(h, "incorrect") || CheckPassword("broken", "anything") {
		t.Fatal("password verification")
	}
	if h == HashPassword("a-strong-password") {
		t.Fatal("salt reused")
	}
}
func TestDomain(t *testing.T) {
	for _, v := range []struct{ in, out string }{{" Example.COM. ", "example.com"}, {"bücher.de", "xn--bcher-kva.de"}} {
		got, e := NormalizeDomain(v.in)
		if e != nil || got != v.out {
			t.Fatalf("%q => %q %v", v.in, got, e)
		}
	}
	for _, v := range []string{"https://example.com", "example.com/path", "localhost", "127.0.0.1", "-bad.com", "a..com", "example.com:80", "*.example.com"} {
		if _, e := NormalizeDomain(v); e == nil {
			t.Errorf("accepted %s", v)
		}
	}
}
