package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestPackageStorage(t *testing.T) {
	dir := t.TempDir()
	s, err := NewPackageStorage(dir, "/application/files", 100)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("a", 50)
	got, err := s.SaveReader("../../Eleph ant 1.0.0.apk", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	if got.Size != 50 || got.Sha256 != hex.EncodeToString(sum[:]) || got.PackageType != "apk" || got.Filename != "Eleph_ant_1.0.0.apk" {
		t.Fatalf("%+v", got)
	}
	if !strings.HasPrefix(got.Path, "/application/files/") || strings.Contains(got.Path, "..") {
		t.Fatalf("bad path %s", got.Path)
	}
	if _, err := os.Stat(dir + strings.TrimPrefix(got.Path, "/application/files")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveReader("a.apk", strings.NewReader(strings.Repeat("a", 101))); err == nil {
		t.Fatal("oversize must fail")
	}
	if _, err := s.SaveReader("a.apk", strings.NewReader("")); err == nil {
		t.Fatal("empty must fail")
	}
	if _, err := s.SaveReader("a.sh", strings.NewReader("x")); err == nil {
		t.Fatal("bad type must fail")
	}
	if got, err := s.SaveReader("pkg.TAR.GZ", strings.NewReader("x")); err != nil || got.PackageType != "tar.gz" {
		t.Fatalf("%v %+v", err, got)
	}
}
