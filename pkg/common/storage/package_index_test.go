package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newPS(t *testing.T) *PackageStorage {
	ps, err := NewPackageStorage(t.TempDir(), "/application/files", 0)
	if err != nil {
		t.Fatal(err)
	}
	return ps
}

func shaOf(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func TestNormalizeSha256(t *testing.T) {
	good := shaOf("x")
	if v, ok := NormalizeSha256(" " + strings.ToUpper(good) + " "); !ok || v != good {
		t.Fatalf("uppercase should normalise: %v %q", ok, v)
	}
	for _, bad := range []string{"", "abc", good[:63], good + "0", strings.Repeat("g", 64)} {
		if _, ok := NormalizeSha256(bad); ok {
			t.Fatalf("%q should be invalid", bad)
		}
	}
}

func TestIndexWriteReadAndSizeMismatch(t *testing.T) {
	ps := newPS(t)
	sp, err := ps.SaveReader("app.dmg", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := ps.readIndex(sp.Sha256)
	if err != nil || e.Size != 5 || e.Filename != "app.dmg" || e.PackageType != "dmg" || e.CreateTime == 0 || strings.HasPrefix(e.Path, "/") {
		t.Fatalf("bad index %+v %v", e, err)
	}
	got, ok := ps.Lookup(sp.Sha256, 5)
	if !ok || got.Path != sp.Path {
		t.Fatalf("lookup %+v %v", got, ok)
	}
	if _, ok := ps.Lookup(sp.Sha256, 6); ok {
		t.Fatal("size mismatch must not match")
	}
	if _, err := ps.readIndex(sp.Sha256); err != nil {
		t.Fatal("size mismatch must not delete a valid entry")
	}
	if _, ok := ps.Lookup(shaOf("nope"), 5); ok {
		t.Fatal("unknown hash")
	}
}

func TestStaleCleanup(t *testing.T) {
	ps := newPS(t)
	sp, _ := ps.SaveReader("a.apk", strings.NewReader("data"))
	e, _ := ps.readIndex(sp.Sha256)
	if err := os.Remove(filepath.Join(ps.BaseDir, e.Path)); err != nil {
		t.Fatal(err)
	}
	if _, ok := ps.Lookup(sp.Sha256, 4); ok {
		t.Fatal("missing file must be stale")
	}
	if _, err := os.Stat(ps.indexFile(sp.Sha256)); !os.IsNotExist(err) {
		t.Fatal("stale entry not removed")
	}
	// size changed on disk
	sp2, _ := ps.SaveReader("b.apk", strings.NewReader("data2"))
	e2, _ := ps.readIndex(sp2.Sha256)
	_ = os.WriteFile(filepath.Join(ps.BaseDir, e2.Path), []byte("longer!!"), 0o644)
	if _, ok := ps.Lookup(sp2.Sha256, 5); ok {
		t.Fatal("changed size must be stale")
	}
}

func TestDedupOnUpload(t *testing.T) {
	ps := newPS(t)
	a, _ := ps.SaveReader("v1.zip", strings.NewReader("same"))
	b, err := ps.SaveReader("other-name.zip", strings.NewReader("same"))
	if err != nil {
		t.Fatal(err)
	}
	if !b.Deduplicated || b.Path != a.Path || a.Deduplicated {
		t.Fatalf("dedup failed a=%+v b=%+v", a, b)
	}
	n := 0
	_ = filepath.Walk(ps.BaseDir, func(p string, i os.FileInfo, _ error) error {
		if !i.IsDir() && !strings.Contains(p, IndexDirName) {
			n++
		}
		return nil
	})
	if n != 1 {
		t.Fatalf("want 1 stored file, got %d", n)
	}
	// stale original -> new upload is kept and re-indexed
	e, _ := ps.readIndex(a.Sha256)
	_ = os.Remove(filepath.Join(ps.BaseDir, e.Path))
	c, _ := ps.SaveReader("v1.zip", strings.NewReader("same"))
	if c.Deduplicated || c.Path == a.Path {
		t.Fatalf("should store fresh copy: %+v", c)
	}
	if got, ok := ps.Lookup(a.Sha256, 4); !ok || got.Path != c.Path {
		t.Fatal("re-index failed")
	}
}

func TestBackfill(t *testing.T) {
	ps := newPS(t)
	mk := func(rel, body string) {
		p := filepath.Join(ps.BaseDir, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(body), 0o644)
	}
	mk("20260101/aa/x.dmg", "one")
	mk("20260101/bb/y.dmg", "one") // duplicate content
	mk("20260102/cc/z.exe", "two")
	mk("20260102/cc/readme.txt", "ignored")
	mk(".index/junk.dmg", "ignored")
	r, err := ps.Backfill()
	if err != nil || r.Indexed != 2 || r.Scanned != 3 || r.Skipped != 1 || r.Failed != 0 {
		t.Fatalf("backfill %+v %v", r, err)
	}
	if got, ok := ps.Lookup(shaOf("two"), 3); !ok || got.Path != "/application/files/20260102/cc/z.exe" || got.PackageType != "exe" {
		t.Fatalf("lookup %+v", got)
	}
	r, _ = ps.Backfill() // idempotent, no rehash of indexed files
	if r.Indexed != 0 || r.Skipped != 3 {
		t.Fatalf("second backfill %+v", r)
	}
}

func TestFileHandlerHidesIndex(t *testing.T) {
	ps := newPS(t)
	sp, _ := ps.SaveReader("a.deb", strings.NewReader("pkg"))
	h := ps.FileHandler()
	get := func(p string) int {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		return w.Code
	}
	if c := get(sp.Path); c != 200 {
		t.Fatalf("package should be served, got %d", c)
	}
	for _, p := range []string{"/application/files/.index/" + sp.Sha256 + ".json", "/application/files/.index/",
		"/application/files/", "/application/files/x/../.index/" + sp.Sha256 + ".json"} {
		if c := get(p); c != 404 {
			t.Fatalf("%s should be 404, got %d", p, c)
		}
	}
}
