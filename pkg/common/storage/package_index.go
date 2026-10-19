package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// IndexDirName is the per-BaseDir directory holding the sha256 index used for instant upload (秒传).
// It is never served by FileHandler.
const IndexDirName = ".index"

// PackageIndexEntry is stored at <BaseDir>/.index/<sha256>.json.
type PackageIndexEntry struct {
	Path        string `json:"path"` // relative to BaseDir, slash separated (yyyymmdd/<rand>/<name>)
	Size        int64  `json:"size"`
	Sha256      string `json:"sha256"`
	Filename    string `json:"filename"`
	PackageType string `json:"package_type"`
	CreateTime  int64  `json:"create_time"` // unix ms
}

// BackfillResult summarises a Backfill scan.
type BackfillResult struct {
	Scanned, Indexed, Skipped, Failed int
}

// NormalizeSha256 trims and lowercases s and reports whether it is 64 hex chars.
func NormalizeSha256(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) != 64 {
		return "", false
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", false
	}
	return s, true
}

func (s *PackageStorage) indexDir() string { return filepath.Join(s.BaseDir, IndexDirName) }

func (s *PackageStorage) indexFile(sha string) string {
	return filepath.Join(s.indexDir(), sha+".json")
}

func (s *PackageStorage) toSaved(e *PackageIndexEntry) *SavedPackage {
	return &SavedPackage{Path: path.Join(s.URLPrefix, e.Path), Size: e.Size, Sha256: e.Sha256,
		Filename: e.Filename, PackageType: e.PackageType}
}

// writeIndex writes the entry atomically (temp file + rename).
func (s *PackageStorage) writeIndex(e *PackageIndexEntry) error {
	if err := os.MkdirAll(s.indexDir(), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.indexDir(), "."+e.Sha256+".*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), s.indexFile(e.Sha256))
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
	}
	return werr
}

func (s *PackageStorage) readIndex(sha string) (*PackageIndexEntry, error) {
	b, err := os.ReadFile(s.indexFile(sha))
	if err != nil {
		return nil, err
	}
	var e PackageIndexEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// lookupEntry returns the live index entry for sha (any size); stale/corrupt entries are removed.
func (s *PackageStorage) lookupEntry(sha string) *PackageIndexEntry {
	e, err := s.readIndex(sha)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			_ = os.Remove(s.indexFile(sha)) // corrupt
		}
		return nil
	}
	clean := path.Clean("/" + e.Path)[1:]
	if e.Sha256 != sha || clean == "" || strings.HasPrefix(clean, IndexDirName) {
		_ = os.Remove(s.indexFile(sha))
		return nil
	}
	st, err := os.Stat(filepath.Join(s.BaseDir, filepath.FromSlash(clean)))
	if err != nil || !st.Mode().IsRegular() || st.Size() != e.Size {
		_ = os.Remove(s.indexFile(sha))
		return nil
	}
	return e
}

// Lookup finds a stored package with this sha256 (already normalised) AND size.
// A stale entry (file gone or size changed on disk) is deleted and reported as not found.
func (s *PackageStorage) Lookup(sha string, size int64) (*SavedPackage, bool) {
	e := s.lookupEntry(sha)
	if e == nil || e.Size != size {
		return nil, false
	}
	return s.toSaved(e), true
}

// Backfill indexes existing package files not yet in the index. Dot-directories (including
// .index) are skipped; when two files share a hash the first one indexed wins.
func (s *PackageStorage) Backfill() (BackfillResult, error) {
	var r BackfillResult
	indexed := map[string]bool{}
	if ents, err := os.ReadDir(s.indexDir()); err == nil {
		for _, de := range ents {
			name := de.Name()
			if !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") {
				continue
			}
			if e := s.lookupEntry(strings.TrimSuffix(name, ".json")); e != nil {
				indexed[e.Path] = true
			}
		}
	}
	err := filepath.WalkDir(s.BaseDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == s.BaseDir {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		pt := PackageTypeOf(d.Name())
		if pt == "" {
			return nil
		}
		r.Scanned++
		rel, rerr := filepath.Rel(s.BaseDir, p)
		if rerr != nil {
			r.Failed++
			return nil
		}
		rel = filepath.ToSlash(rel)
		if indexed[rel] {
			r.Skipped++
			return nil
		}
		sha, size, herr := hashFile(p)
		if herr != nil {
			r.Failed++
			return nil
		}
		if e := s.lookupEntry(sha); e != nil {
			r.Skipped++ // duplicate content already indexed under another path
			return nil
		}
		var ct int64
		if info, ierr := d.Info(); ierr == nil {
			ct = info.ModTime().UnixMilli()
		}
		if werr := s.writeIndex(&PackageIndexEntry{Path: rel, Size: size, Sha256: sha, Filename: d.Name(), PackageType: pt, CreateTime: ct}); werr != nil {
			r.Failed++
			return nil
		}
		indexed[rel] = true
		r.Indexed++
		return nil
	})
	return r, err
}

func hashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// FileHandler serves BaseDir like http.FileServer but returns 404 for any path segment starting
// with "." (so .index and temp files are never exposed) and for directory listings.
func (s *PackageStorage) FileHandler() http.Handler {
	fsrv := http.StripPrefix(s.URLPrefix, http.FileServer(http.Dir(s.BaseDir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, s.URLPrefix)
		for _, seg := range strings.Split(rest, "/") {
			if strings.HasPrefix(seg, ".") {
				http.NotFound(w, r)
				return
			}
		}
		if rest == "" || strings.HasSuffix(rest, "/") {
			http.NotFound(w, r)
			return
		}
		if st, err := os.Stat(filepath.Join(s.BaseDir, filepath.FromSlash(path.Clean("/"+rest)))); err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		fsrv.ServeHTTP(w, r)
	})
}
