package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DefaultMaxPackageSize caps one uploaded client package (1 GiB).
const DefaultMaxPackageSize int64 = 1 << 30

var packageSuffixes = map[string]string{ // suffix -> package_type
	".apk": "apk", ".exe": "exe", ".dmg": "dmg", ".zip": "zip", ".deb": "deb", ".ipa": "ipa", ".tar.gz": "tar.gz",
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// PackageStorage stores client installation packages on local disk (same mechanism as
// AppealStorage: files live under BaseDir and are served by a gin Static route at URLPrefix).
type PackageStorage struct {
	BaseDir     string
	URLPrefix   string // e.g. /application/files
	MaxFileSize int64
}

// SavedPackage describes a stored package.
type SavedPackage struct {
	Path        string // URL path, URLPrefix + /yyyymmdd/<rand>/<name>
	Size        int64
	Sha256      string
	Filename    string
	PackageType string
}

func NewPackageStorage(baseDir, urlPrefix string, maxFileSize int64) (*PackageStorage, error) {
	if baseDir == "" {
		baseDir = "./app_packages"
	}
	if urlPrefix == "" {
		urlPrefix = "/application/files"
	}
	if maxFileSize <= 0 {
		maxFileSize = DefaultMaxPackageSize
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	return &PackageStorage{BaseDir: baseDir, URLPrefix: urlPrefix, MaxFileSize: maxFileSize}, nil
}

// PackageTypeOf returns the package_type for a file name, or "" when unsupported.
func PackageTypeOf(name string) string {
	l := strings.ToLower(name)
	for suf, t := range packageSuffixes {
		if strings.HasSuffix(l, suf) {
			return t
		}
	}
	return ""
}

// Save streams the upload to disk while computing size and sha256.
func (s *PackageStorage) Save(fh *multipart.FileHeader) (*SavedPackage, error) {
	src, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	return s.SaveReader(fh.Filename, src)
}

func (s *PackageStorage) SaveReader(filename string, src io.Reader) (*SavedPackage, error) {
	base := filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	pt := PackageTypeOf(base)
	if pt == "" {
		return nil, errors.New("unsupported file type, want apk|exe|dmg|zip|deb|ipa|tar.gz")
	}
	name := unsafeName.ReplaceAllString(base, "_")
	rnd := make([]byte, 8)
	if _, err := rand.Read(rnd); err != nil {
		return nil, err
	}
	rel := path.Join(time.Now().Format("20060102"), hex.EncodeToString(rnd))
	dir := filepath.Join(s.BaseDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dstPath := filepath.Join(dir, name)
	dst, err := os.Create(dstPath)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(src, s.MaxFileSize+1))
	cerr := dst.Close()
	if err == nil {
		err = cerr
	}
	if err == nil && (n <= 0 || n > s.MaxFileSize) {
		err = errors.New("file size out of range")
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &SavedPackage{
		Path: path.Join(s.URLPrefix, rel, name), Size: n, Sha256: hex.EncodeToString(h.Sum(nil)),
		Filename: name, PackageType: pt,
	}, nil
}
