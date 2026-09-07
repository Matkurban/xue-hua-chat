package chat

import (
	"strings"
	"time"

	admindb "chat/pkg/common/db/table/admin"
	"chat/pkg/common/mctx"
	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ReportApi 用户侧举报 API
type ReportApi struct {
	report admindb.ReportInterface
}

func NewReportApi(report admindb.ReportInterface) *ReportApi {
	return &ReportApi{report: report}
}

type CreateReportReq struct {
	TargetType   string   `json:"targetType" binding:"required,oneof=user group message"`
	TargetID     string   `json:"targetID" binding:"required"`
	MessageID    string   `json:"messageID"`
	Category     string   `json:"category" binding:"required,oneof=harass scam porn ad other"`
	Description  string   `json:"description"`
	EvidenceUrls []string `json:"evidenceUrls"`
}

type CreateReportResp struct {
	ReportID string `json:"reportID"`
}

func (o *ReportApi) CreateReport(c *gin.Context) {
	var req CreateReportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	reporterID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if req.TargetType == "message" && strings.TrimSpace(req.MessageID) == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("messageID required for message report"))
		return
	}
	if len(req.EvidenceUrls) > 5 {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("too many evidence files"))
		return
	}
	if reporterID == req.TargetID && req.TargetType == "user" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("cannot report self"))
		return
	}
	r := &admindb.Report{
		ReportID:    uuid.NewString(),
		ReporterID:  reporterID,
		TargetType:  req.TargetType,
		TargetID:    req.TargetID,
		MessageID:   req.MessageID,
		Category:    req.Category,
		Description: req.Description,
		Evidence:    req.EvidenceUrls,
		Status:      "pending",
		CreateTime:  time.Now(),
	}
	if err := o.report.Create(c, []*admindb.Report{r}); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &CreateReportResp{ReportID: r.ReportID})
}
