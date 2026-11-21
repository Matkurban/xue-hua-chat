package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
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

var allowedImageMIME = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
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
	if appealID == "" || filepath.Base(appealID) != appealID || appealID == "." || appealID == ".." || strings.ContainsAny(appealID, `/\`) {
		return "", errors.New("invalid appeal id")
	}
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

	// 校验文件真实内容魔数，防止伪造扩展名上传非图片文件
	header := make([]byte, 512)
	n, readErr := io.ReadFull(src, header)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return "", readErr
	}
	if n == 0 {
		return "", errors.New("empty file")
	}
	mimeType := http.DetectContentType(header[:n])
	if !allowedImageMIME[mimeType] {
		return "", errors.New("unsupported image content")
	}

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
	reader := io.MultiReader(bytes.NewReader(header[:n]), src)
	written, copyErr := io.Copy(dst, io.LimitReader(reader, s.MaxFileSize+1))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil || written > s.MaxFileSize {
		_ = os.Remove(dstPath)
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return "", errors.New("file size out of range")
	}
	return path.Join(s.URLPrefix, day, appealID, name), nil
}
