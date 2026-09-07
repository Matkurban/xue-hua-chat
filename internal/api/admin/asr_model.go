package admin

import (
	"time"

	admindb "chat/pkg/common/db/table/admin"
	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"github.com/gin-gonic/gin"
)

var asrExpectedFiles = map[string][]string{
	"sense_voice_int8":  {"model.int8.onnx", "tokens.txt"},
	"whisper_tiny_int8": {"tiny-encoder.int8.onnx", "tiny-decoder.int8.onnx", "tiny-tokens.txt"},
}

type AsrModelApi struct {
	store admindb.AsrModelInterface
}

func NewAsrModelApi(store admindb.AsrModelInterface) *AsrModelApi {
	return &AsrModelApi{store: store}
}

type asrModelListResp struct {
	List []*admindb.AsrModel `json:"list"`
}

func (o *AsrModelApi) List(c *gin.Context) {
	if o == nil || o.store == nil {
		apiresp.GinSuccess(c, &asrModelListResp{List: []*admindb.AsrModel{}})
		return
	}
	list, err := o.store.List(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if list == nil {
		list = []*admindb.AsrModel{}
	}
	apiresp.GinSuccess(c, &asrModelListResp{List: list})
}

func (o *AsrModelApi) Upsert(c *gin.Context) {
	if o == nil || o.store == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("asr model store unavailable"))
		return
	}
	var req admindb.AsrModel
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if err := validateAsrModel(&req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.UpdateTime = time.Now()
	if err := o.store.Upsert(c, &req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, nil)
}

func validateAsrModel(req *admindb.AsrModel) error {
	expected, ok := asrExpectedFiles[req.ID]
	if !ok {
		return errs.ErrArgs.WrapMsg("unsupported asr model id")
	}
	if req.Version == "" {
		return errs.ErrArgs.WrapMsg("version is required")
	}
	if len(req.Files) != len(expected) {
		return errs.ErrArgs.WrapMsg("file list does not match model")
	}
	seen := make(map[string]admindb.AsrModelFile, len(req.Files))
	for _, file := range req.Files {
		if file.FileName == "" || file.Url == "" || file.Sha256 == "" || file.MinBytes <= 0 {
			return errs.ErrArgs.WrapMsg("each file needs fileName, url, sha256 and minBytes")
		}
		seen[file.FileName] = file
	}
	for _, name := range expected {
		if _, ok := seen[name]; !ok {
			return errs.ErrArgs.WrapMsg("missing file " + name)
		}
	}
	return nil
}
