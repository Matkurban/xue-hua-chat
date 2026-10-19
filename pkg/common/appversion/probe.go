package appversion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"syscall"
	"time"
)

const (
	// MaxPackageSize caps both uploads and probe downloads (1 GiB).
	MaxPackageSize int64 = 1 << 30
	// ProbeTimeout bounds one whole probe (connect + redirects + body).
	ProbeTimeout = 5 * time.Minute
	maxRedirects = 5
)

// ProbeResult is the metadata of a remote package.
type ProbeResult struct {
	Size     int64  `json:"size"`
	Sha256   string `json:"sha256"`
	Filename string `json:"filename"`
}

var blockedNets = mustCIDRs(
	"0.0.0.0/8",     // "this" network
	"100.64.0.0/10", // CGNAT / some cloud metadata (100.100.100.200)
	"192.0.0.0/24",  // IETF protocol assignments
	"198.18.0.0/15", // benchmarking
	"240.0.0.0/4",   // reserved
	"64:ff9b::/96",  // NAT64
)

func mustCIDRs(list ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(list))
	for _, c := range list {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

// IsBlockedIP reports whether an address must never be fetched by the server
// (loopback, private, link-local incl. cloud metadata 169.254.169.254, multicast, unspecified, ...).
func IsBlockedIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4 // unwrap IPv4-mapped IPv6
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Prober fetches package metadata from a remote URL in an SSRF-safe way:
// only http/https, every connection (initial and after each redirect) is checked against
// IsBlockedIP on the *resolved* address at dial time (defeats DNS rebinding), redirects are capped,
// proxies are ignored, the body size and total time are capped.
type Prober struct {
	maxSize int64
	timeout time.Duration
	// allowIP overrides IsBlockedIP; tests only.
	allowIP func(net.IP) bool
}

// NewProber returns a Prober with the production limits.
func NewProber() *Prober { return &Prober{maxSize: MaxPackageSize, timeout: ProbeTimeout} }

func (p *Prober) blocked(ip net.IP) bool {
	if p.allowIP != nil {
		return !p.allowIP(ip)
	}
	return IsBlockedIP(ip)
}

func (p *Prober) client() *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || p.blocked(ip) {
				return fmt.Errorf("address %s is not allowed", host)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: p.timeout,
		Transport: &http.Transport{
			Proxy:                 nil, // never go through an env proxy: it would bypass the dial check
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			return checkScheme(req.URL)
		},
	}
}

func checkScheme(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https urls are allowed, got %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return errors.New("url has no host")
	}
	if u.User != nil {
		return errors.New("url must not contain credentials")
	}
	return nil
}

// Probe downloads the resource (streaming, never to disk) and returns size, sha256 and file name.
func (p *Prober) Probe(ctx context.Context, rawURL string) (*ProbeResult, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if err := checkScheme(u); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "xue-hua-chat-probe/1")
	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote returned status %d", resp.StatusCode)
	}
	if resp.ContentLength > p.maxSize {
		return nil, fmt.Errorf("file too large (limit %d bytes)", p.maxSize)
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(resp.Body, p.maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	if n > p.maxSize {
		return nil, fmt.Errorf("file too large (limit %d bytes)", p.maxSize)
	}
	return &ProbeResult{Size: n, Sha256: hex.EncodeToString(h.Sum(nil)), Filename: filenameOf(resp, resp.Request.URL)}, nil
}

func filenameOf(resp *http.Response, u *url.URL) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil && params["filename"] != "" {
			return path.Base(params["filename"])
		}
	}
	b := path.Base(u.Path)
	if b == "." || b == "/" {
		return ""
	}
	return b
}
