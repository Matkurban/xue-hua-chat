// Package appversion holds the pure rules of the client self-update feature:
// parsing/comparing versions, validating releases, and deciding whether a client must update.
package appversion

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	admindb "chat/pkg/common/db/table/admin"
)

const (
	StatusPublished = "published"
	StatusRevoked   = "revoked"

	// ArchUniversal matches every client architecture (fallback when no exact-arch record exists).
	ArchUniversal = "universal"
)

var (
	platforms    = map[string]bool{"android": true, "ios": true, "windows": true, "macos": true, "linux": true}
	arches       = map[string]bool{ArchUniversal: true, "arm64": true, "x64": true, "armeabi-v7a": true, "amd64": true}
	packageTypes = map[string]bool{"apk": true, "exe": true, "dmg": true, "zip": true, "deb": true, "ipa": true, "tar.gz": true}
	statuses     = map[string]bool{StatusPublished: true, StatusRevoked: true}
	versionRe    = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	sha256Re     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Version is a parsed x.y.z version.
type Version [3]int64

// ParseVersion parses "x.y.z" (an optional leading "v" and a "+build" suffix are tolerated).
func ParseVersion(s string) (Version, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	if !versionRe.MatchString(s) {
		return Version{}, fmt.Errorf("invalid version %q, want x.y.z", s)
	}
	var v Version
	for i, p := range strings.Split(s, ".") {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version %q", s)
		}
		v[i] = n
	}
	return v, nil
}

// CompareVersion returns -1, 0 or 1.
func CompareVersion(a, b Version) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// Compare orders (version, build) pairs; build only breaks ties.
func Compare(av Version, ab int64, bv Version, bb int64) int {
	if c := CompareVersion(av, bv); c != 0 {
		return c
	}
	switch {
	case ab < bb:
		return -1
	case ab > bb:
		return 1
	}
	return 0
}

// Validate checks (and normalises Arch/Status/Sha256/PackageType/Url of) a release before it is stored.
func Validate(a *admindb.Application) error {
	if !platforms[a.Platform] {
		return fmt.Errorf("invalid platform %q", a.Platform)
	}
	if a.Arch == "" {
		a.Arch = ArchUniversal
	}
	if !arches[a.Arch] {
		return fmt.Errorf("invalid arch %q", a.Arch)
	}
	if !versionRe.MatchString(a.Version) { // stored versions must be canonical x.y.z
		return fmt.Errorf("invalid version %q, want x.y.z", a.Version)
	}
	v, _ := ParseVersion(a.Version)
	if a.Build < 0 {
		return errors.New("build must be >= 0")
	}
	if a.MinVersion != "" {
		if !versionRe.MatchString(a.MinVersion) {
			return fmt.Errorf("invalid min_version %q, want x.y.z", a.MinVersion)
		}
		mv, _ := ParseVersion(a.MinVersion)
		if CompareVersion(mv, v) > 0 {
			return errors.New("min_version must not be greater than version")
		}
	}
	if a.Status == "" {
		a.Status = StatusPublished
	}
	if !statuses[a.Status] {
		return fmt.Errorf("invalid status %q", a.Status)
	}
	a.Sha256 = strings.ToLower(strings.TrimSpace(a.Sha256))
	a.PackageType = strings.ToLower(strings.TrimSpace(a.PackageType))
	a.Url = strings.TrimSpace(a.Url)
	if a.PackageType != "" && !packageTypes[a.PackageType] {
		return fmt.Errorf("invalid package_type %q", a.PackageType)
	}
	if a.Size < 0 {
		return errors.New("size must be >= 0")
	}
	if a.Sha256 != "" && !sha256Re.MatchString(a.Sha256) {
		return errors.New("sha256 must be 64 hex chars")
	}
	if a.Url != "" {
		if err := checkURL(a.Url); err != nil {
			return err
		}
	}
	return nil
}

func checkURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("url must be a valid http(s) URL, got %q", s)
	}
	return nil
}

// Result is the outcome of a client check.
type Result struct {
	Latest    *admindb.Application // newest matching release; nil when there is none
	HasUpdate bool
	Force     bool
}

// Check decides what a client at (clientVersion, clientBuild, arch) must do, given the published
// releases of its platform.
//
//   - candidates are published releases whose arch equals the client's arch or "universal"
//     (on equal (version, build) the exact-arch record wins);
//   - Latest is the highest (version, build) candidate; HasUpdate means it is newer than the client;
//   - Force (only when HasUpdate) = Latest.Force OR client version < Latest.MinVersion.
//     Nothing is forced when there is no record or the client is current.
func Check(releases []*admindb.Application, clientVersion string, clientBuild int64, arch string) (*Result, error) {
	cv, err := ParseVersion(clientVersion)
	if err != nil {
		return nil, err
	}
	var best *admindb.Application
	var bestV Version
	for _, r := range releases {
		if r.Status != StatusPublished || (r.Arch != ArchUniversal && r.Arch != arch) {
			continue
		}
		rv, err := ParseVersion(r.Version)
		if err != nil {
			continue // ignore corrupt legacy records
		}
		if best == nil {
			best, bestV = r, rv
			continue
		}
		c := Compare(rv, r.Build, bestV, best.Build)
		if c > 0 || (c == 0 && best.Arch == ArchUniversal && r.Arch != ArchUniversal) {
			best, bestV = r, rv
		}
	}
	res := &Result{Latest: best}
	if best == nil || Compare(bestV, best.Build, cv, clientBuild) <= 0 {
		return res, nil
	}
	res.HasUpdate = true
	res.Force = best.Force
	if best.MinVersion != "" {
		if mv, err := ParseVersion(best.MinVersion); err == nil && CompareVersion(cv, mv) < 0 {
			res.Force = true
		}
	}
	return res, nil
}
