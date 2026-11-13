package integrations

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSignatureEncryptionAndSSRF(t *testing.T) {
	body := []byte(`{"event_id":"test"}`)
	now := time.Unix(123456, 0)
	sig := Signature("secret", "123456", body)
	if !Verify("secret", "123456", sig, body, now) || Verify("other", "123456", sig, body, now) || Verify("secret", "123456", sig, []byte("bad"), now) || Verify("secret", "123456", sig, body, now.Add(6*time.Minute)) {
		t.Fatal("signature/replay")
	}
	key := bytes.Repeat([]byte{1}, 32)
	b, e := seal(key, "org", "id", "secret")
	if e != nil {
		t.Fatal(e)
	}
	v, e := open(key, "org", "id", b)
	if e != nil || v != "secret" {
		t.Fatal(e)
	}
	if _, e = open(key, "foreign", "id", b); e == nil {
		t.Fatal("cipher scope")
	}
	c := Config{}
	for _, u := range []string{"http://example.com", "https://localhost", "https://127.0.0.1", "https://10.0.0.1", "https://169.254.169.254", "https://168.63.129.16", "https://[fec0::1]", "https://[::1]", "https://user:pass@example.com", "https://example.com?secret=x"} {
		if c.ValidateURL(u) == nil {
			t.Error("unsafe URL", u)
		}
	}
	// DNS rebinding/internal DNS is rejected by the actual pinned dialer.
	client := c.Client("https://127.0.0.1.nip.io")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "POST", "https://localhost.localdomain", nil)
	if resp, e := client.Do(r); e == nil {
		resp.Body.Close()
		t.Fatal("internal DNS accepted")
	}
	var reached bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer source.Close()
	c.DevReceiver = source.URL
	resp, e := c.Client(source.URL).Post(source.URL, "application/json", bytes.NewReader(body))
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if reached || resp.StatusCode != 302 {
		t.Fatal("redirect followed")
	}
	t.Setenv("WEBHOOK_DEV_RECEIVER_URL", source.URL)
	if _, e = Load(true); e == nil {
		t.Fatal("production local receiver")
	}
}
