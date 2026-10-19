package appversion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "0.0.0.0", "100.100.100.200",
		"fe80::1", "fc00::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "224.0.0.1", "198.18.0.1", "255.255.255.255"}
	for _, s := range blocked {
		if !IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111", "93.184.216.34"} {
		if IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func testProber() *Prober {
	return &Prober{maxSize: 1024, timeout: 5 * time.Second, allowIP: func(net.IP) bool { return true }}
}

func TestProbeOK(t *testing.T) {
	body := strings.Repeat("x", 500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redir" {
			http.Redirect(w, r, "/files/app.apk", http.StatusFound)
			return
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	sum := sha256.Sum256([]byte(body))
	res, err := testProber().Probe(context.Background(), srv.URL+"/redir")
	if err != nil {
		t.Fatal(err)
	}
	if res.Size != 500 || res.Sha256 != hex.EncodeToString(sum[:]) || res.Filename != "app.apk" {
		t.Fatalf("%+v", res)
	}
}

func TestProbeContentDisposition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="Elephant-1.0.0.dmg"`)
		w.Write([]byte("abc"))
	}))
	defer srv.Close()
	res, err := testProber().Probe(context.Background(), srv.URL+"/dl?id=1")
	if err != nil || res.Filename != "Elephant-1.0.0.dmg" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestProbeRejects(t *testing.T) {
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 5000)))
	}))
	defer big.Close()
	chunked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := w.(http.Flusher)
		for i := 0; i < 10; i++ {
			w.Write([]byte(strings.Repeat("x", 500)))
			f.Flush()
		}
	}))
	defer chunked.Close()
	notFound := httptest.NewServer(http.NotFoundHandler())
	defer notFound.Close()
	p := testProber()
	for name, u := range map[string]string{
		"too large (content-length)": big.URL, "too large (chunked)": chunked.URL, "404": notFound.URL,
		"file scheme": "file:///etc/passwd", "ftp": "ftp://example.com/a", "no host": "http://", "creds": "http://a:b@example.com/",
		"garbage": "::::",
	} {
		if _, err := p.Probe(context.Background(), u); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestProbeSSRF(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true; w.Write([]byte("secret")) }))
	defer srv.Close() // listens on 127.0.0.1
	p := NewProber()  // production rules
	p.timeout = 5 * time.Second
	if _, err := p.Probe(context.Background(), srv.URL); err == nil || hit {
		t.Fatalf("loopback must be blocked (hit=%v err=%v)", hit, err)
	}
	if _, err := p.Probe(context.Background(), "http://localhost:1/"); err == nil {
		t.Fatal("localhost must be blocked")
	}
	if _, err := p.Probe(context.Background(), "http://169.254.169.254/latest/meta-data/"); err == nil {
		t.Fatal("metadata must be blocked")
	}
	if _, err := p.Probe(context.Background(), "http://[::1]:1/"); err == nil {
		t.Fatal("::1 must be blocked")
	}
	// public host redirecting to loopback: emulate by redirect from an allowed server to a blocked IP
	allowOnly := func(ip net.IP) bool { return false }
	hit = false
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, srv.URL, http.StatusFound) }))
	defer redir.Close()
	q := &Prober{maxSize: 1024, timeout: 5 * time.Second, allowIP: allowOnly}
	if _, err := q.Probe(context.Background(), redir.URL); err == nil || hit {
		t.Fatal("blocked after redirect")
	}
	// redirect to non-http scheme
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer bad.Close()
	if _, err := testProber().Probe(context.Background(), bad.URL); err == nil {
		t.Fatal("redirect to file:// must fail")
	}
}
