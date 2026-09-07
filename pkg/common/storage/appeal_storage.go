package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultMaxFileSize = 5 * 1024 * 1024 // 5MB
	DefaultMaxFiles    = 5
)

var allowedExt = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
}

// AppealStorage 申诉证据本地文件存储
type AppealStorage struct {
	BaseDir     string // 物理目录
	URLPrefix   string // 对外访问前缀，如 /appeal/files
	MaxFileSize int64
}

func NewAppealStorage(baseDir, urlPrefix string, maxFileSize int64) (*AppealStorage, error) {
	if baseDir == "" {
		baseDir = "./appeal_evidence"
	}
	if urlPrefix == "" {
		urlPrefix = "/appeal/files"
	}
	if maxFileSize <= 0 {
		maxFileSize = DefaultMaxFileSize
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	return &AppealStorage{BaseDir: baseDir, URLPrefix: urlPrefix, MaxFileSize: maxFileSize}, nil
}

// Save 保存上传的图片，返回外部可访问的 URL（相对路径）
func (s *AppealStorage) Save(appealID string, fh *multipart.FileHeader) (string, error) {
	if fh.Size <= 0 || fh.Size > s.MaxFileSize {
		return "", errors.New("file size out of range")
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !allowedExt[ext] {
		return "", errors.New("unsupported file type")
	}
	src, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	day := time.Now().Format("20060102")
	dir := filepath.Join(s.BaseDir, day, appealID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	rnd := make([]byte, 8)
	if _, err := rand.Read(rnd); err != nil {
		return "", err
	}
	name := hex.EncodeToString(rnd) + ext
	dstPath := filepath.Join(dir, name)
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return path.Join(s.URLPrefix, day, appealID, name), nil
}
