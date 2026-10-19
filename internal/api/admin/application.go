package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"github.com/gin-gonic/gin"
)

// UploadApplicationPackageResp is the result of POST /application/upload.
type UploadApplicationPackageResp struct {
	Url         string `json:"url"`
	Size        int64  `json:"size"`
	Sha256      string `json:"sha256"`
	Filename    string `json:"filename"`
	PackageType string `json:"package_type"`
}

// UploadApplicationPackage stores a client package (multipart field "file") and returns its
// download url plus size and sha256 computed server-side. Admin only.
func (o *Api) UploadApplicationPackage(c *gin.Context) {
	if o.Packages == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("package storage is not available"))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, o.Packages.MaxFileSize+(1<<20))
	mr, err := c.Request.MultipartReader()
	if err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("multipart/form-data with a 'file' field is required"))
		return
	}
	for {
		part, err := mr.NextPart() // streamed: the package is never fully buffered in memory
		if err != nil {
			apiresp.GinError(c, errs.ErrArgs.WrapMsg("missing 'file' field"))
			return
		}
		if part.FormName() != "file" || part.FileName() == "" {
			continue
		}
		saved, err := o.Packages.SaveReader(part.FileName(), part)
		if err != nil {
			apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
			return
		}
		apiresp.GinSuccess(c, &UploadApplicationPackageResp{
			Url: o.packageBaseURL(c) + saved.Path, Size: saved.Size, Sha256: saved.Sha256,
			Filename: saved.Filename, PackageType: saved.PackageType,
		})
		return
	}
}

// packageBaseURL is the public origin used to build absolute download urls.
// appPackage.baseURL (chat-api-admin.yml) is used when set (recommended behind a reverse proxy), else it is derived from the request.
func (o *Api) packageBaseURL(c *gin.Context) string {
	if o.PackageBaseURL != "" {
		return o.PackageBaseURL
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if v := c.GetHeader("X-Forwarded-Proto"); v != "" {
		scheme = strings.TrimSpace(strings.Split(v, ",")[0])
	}
	host := c.Request.Host
	if v := c.GetHeader("X-Forwarded-Host"); v != "" {
		host = strings.TrimSpace(strings.Split(v, ",")[0])
	}
	return scheme + "://" + host
}

type probeURLReq struct {
	Url string `json:"url"`
}

// ProbeApplicationURL fetches size/sha256/filename of a remote package (SSRF-safe). Admin only.
func (o *Api) ProbeApplicationURL(c *gin.Context) {
	var req probeURLReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Url) == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("url is required"))
		return
	}
	res, err := o.Prober.Probe(c.Request.Context(), strings.TrimSpace(req.Url))
	if err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, res)
}

// unwrapValueFields lets update_version accept both {"text":"x"} and the wrapper form
// {"text":{"value":"x"}}: the protobuf wrapper types decode plain JSON values only, so any
// top-level field whose value is an object with a single "value" key is replaced by that value.
func unwrapValueFields(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return
	}
	restore := func(b []byte) { c.Request.Body = io.NopCloser(bytes.NewReader(b)) }
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		restore(raw)
		return
	}
	for k, v := range m {
		var w map[string]json.RawMessage
		if json.Unmarshal(v, &w) == nil && len(w) == 1 {
			if inner, ok := w["value"]; ok {
				m[k] = inner
			}
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		restore(raw)
		return
	}
	restore(out)
}
