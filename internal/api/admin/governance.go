package admin

import (
	"strings"
	"time"

	chatmw "chat/internal/api/mw"
	"chat/pkg/common/captcha"
	admindb "chat/pkg/common/db/table/admin"
	"chat/pkg/common/mctx"
	"chat/pkg/common/storage"
	"chat/pkg/protocol/admin"
	"chat/pkg/common/apiresp"
	"chat/pkg/common/errs"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/openimsdk/protocol/sdkws"
)

// GovernanceApi 举报 + 申诉 管理端 + 公开端 API
type GovernanceApi struct {
	report       admindb.ReportInterface
	appeal       admindb.AppealInterface
	captchaSvc   *captcha.Service
	AppealTokens *chatmw.AppealTokenManager
	storage      *storage.AppealStorage
	adminClient  admin.AdminClient
}

func NewGovernanceApi(
	report admindb.ReportInterface,
	appeal admindb.AppealInterface,
	cap *captcha.Service,
	tokens *chatmw.AppealTokenManager,
	st *storage.AppealStorage,
	adminClient admin.AdminClient,
) *GovernanceApi {
	return &GovernanceApi{
		report:       report,
		appeal:       appeal,
		captchaSvc:   cap,
		AppealTokens: tokens,
		storage:      st,
		adminClient:  adminClient,
	}
}

// ─── 公共 DTO ─────────────────────────────────────────────────────────────

type pageReq struct {
	PageNumber int32 `json:"pageNumber"`
	ShowNumber int32 `json:"showNumber"`
}

func (p pageReq) toPagination() *sdkws.RequestPagination {
	page := p.PageNumber
	show := p.ShowNumber
	if page <= 0 {
		page = 1
	}
	if show <= 0 || show > 200 {
		show = 20
	}
	return &sdkws.RequestPagination{PageNumber: page, ShowNumber: show}
}

// ─── Report (admin) ───────────────────────────────────────────────────────

type SearchReportReq struct {
	pageReq
	Status   string `json:"status"`
	Category string `json:"category"`
	Keyword  string `json:"keyword"`
}

type ReportItem struct {
	ReportID     string   `json:"reportID"`
	ReporterID   string   `json:"reporterID"`
	TargetType   string   `json:"targetType"`
	TargetID     string   `json:"targetID"`
	MessageID    string   `json:"messageID,omitempty"`
	Category     string   `json:"category"`
	Description  string   `json:"description"`
	Evidence     []string `json:"evidence"`
	Status       string   `json:"status"`
	HandlerID    string   `json:"handlerID,omitempty"`
	HandleNote   string   `json:"handleNote,omitempty"`
	HandleAction string   `json:"handleAction,omitempty"`
	CreateTime   int64    `json:"createTime"`
	HandleTime   int64    `json:"handleTime,omitempty"`
}

type SearchReportResp struct {
	Total uint32        `json:"total"`
	List  []*ReportItem `json:"list"`
}

func reportToItem(r *admindb.Report) *ReportItem {
	item := &ReportItem{
		ReportID:     r.ReportID,
		ReporterID:   r.ReporterID,
		TargetType:   r.TargetType,
		TargetID:     r.TargetID,
		MessageID:    r.MessageID,
		Category:     r.Category,
		Description:  r.Description,
		Evidence:     r.Evidence,
		Status:       r.Status,
		HandlerID:    r.HandlerID,
		HandleNote:   r.HandleNote,
		HandleAction: r.HandleAction,
		CreateTime:   r.CreateTime.UnixMilli(),
	}
	if !r.HandleTime.IsZero() {
		item.HandleTime = r.HandleTime.UnixMilli()
	}
	if item.Evidence == nil {
		item.Evidence = []string{}
	}
	return item
}

func (o *GovernanceApi) SearchReport(c *gin.Context) {
	var req SearchReportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if _, err := mctx.CheckAdmin(c); err != nil {
		apiresp.GinError(c, err)
		return
	}
	total, items, err := o.report.Search(c, req.Status, req.Category, req.Keyword, req.toPagination())
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	out := make([]*ReportItem, 0, len(items))
	for _, it := range items {
		out = append(out, reportToItem(it))
	}
	apiresp.GinSuccess(c, &SearchReportResp{Total: uint32(total), List: out})
}

type ReportDetailReq struct {
	ReportID string `json:"reportID" binding:"required"`
}

func (o *GovernanceApi) ReportDetail(c *gin.Context) {
	var req ReportDetailReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if _, err := mctx.CheckAdmin(c); err != nil {
		apiresp.GinError(c, err)
		return
	}
	r, err := o.report.Take(c, req.ReportID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, reportToItem(r))
}

type HandleReportReq struct {
	ReportID string `json:"reportID" binding:"required"`
	Action   string `json:"action" binding:"required,oneof=ignore resolved block_user"`
	Note     string `json:"note"`
	// 当 action=block_user 时
	BlockReason string `json:"blockReason"`
}

func (o *GovernanceApi) HandleReport(c *gin.Context) {
	var req HandleReportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	adminID, err := mctx.CheckAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	r, err := o.report.Take(c, req.ReportID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	status := "resolved"
	switch req.Action {
	case "ignore":
		status = "rejected"
	case "block_user":
		if r.TargetType != "user" {
			apiresp.GinError(c, errs.ErrArgs.WrapMsg("target is not a user"))
			return
		}
		reason := req.BlockReason
		if reason == "" {
			reason = req.Note
		}
		if reason == "" {
			reason = "report:" + r.Category
		}
		_, berr := o.adminClient.BlockUser(c, &admin.BlockUserReq{UserID: r.TargetID, Reason: reason})
		if berr != nil {
			apiresp.GinError(c, berr)
			return
		}
	}
	now := time.Now()
	upd := map[string]any{
		"status":        status,
		"handler_id":    adminID,
		"handle_note":   req.Note,
		"handle_action": req.Action,
		"handle_time":   now,
	}
	if err := o.report.Update(c, req.ReportID, upd); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, gin.H{})
}

// ─── Appeal (admin) ───────────────────────────────────────────────────────

type AppealItem struct {
	AppealID    string   `json:"appealID"`
	Account     string   `json:"account"`
	UserID      string   `json:"userID,omitempty"`
	Reason      string   `json:"reason"`
	Description string   `json:"description"`
	Evidence    []string `json:"evidence"`
	Contact     string   `json:"contact,omitempty"`
	Status      string   `json:"status"`
	HandlerID   string   `json:"handlerID,omitempty"`
	HandleNote  string   `json:"handleNote,omitempty"`
	ClientIP    string   `json:"clientIP,omitempty"`
	CreateTime  int64    `json:"createTime"`
	HandleTime  int64    `json:"handleTime,omitempty"`
}

func appealToItem(a *admindb.Appeal) *AppealItem {
	item := &AppealItem{
		AppealID:    a.AppealID,
		Account:     a.Account,
		UserID:      a.UserID,
		Reason:      a.Reason,
		Description: a.Description,
		Evidence:    a.Evidence,
		Contact:     a.Contact,
		Status:      a.Status,
		HandlerID:   a.HandlerID,
		HandleNote:  a.HandleNote,
		ClientIP:    a.ClientIP,
		CreateTime:  a.CreateTime.UnixMilli(),
	}
	if !a.HandleTime.IsZero() {
		item.HandleTime = a.HandleTime.UnixMilli()
	}
	if item.Evidence == nil {
		item.Evidence = []string{}
	}
	return item
}

type SearchAppealReq struct {
	pageReq
	Status  string `json:"status"`
	Keyword string `json:"keyword"`
}

type SearchAppealResp struct {
	Total uint32        `json:"total"`
	List  []*AppealItem `json:"list"`
}

func (o *GovernanceApi) SearchAppeal(c *gin.Context) {
	var req SearchAppealReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if _, err := mctx.CheckAdmin(c); err != nil {
		apiresp.GinError(c, err)
		return
	}
	total, items, err := o.appeal.Search(c, req.Status, req.Keyword, req.toPagination())
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	out := make([]*AppealItem, 0, len(items))
	for _, it := range items {
		out = append(out, appealToItem(it))
	}
	apiresp.GinSuccess(c, &SearchAppealResp{Total: uint32(total), List: out})
}

type AppealDetailReq struct {
	AppealID string `json:"appealID" binding:"required"`
}

func (o *GovernanceApi) AppealDetail(c *gin.Context) {
	var req AppealDetailReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if _, err := mctx.CheckAdmin(c); err != nil {
		apiresp.GinError(c, err)
		return
	}
	a, err := o.appeal.Take(c, req.AppealID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, appealToItem(a))
}

type HandleAppealReq struct {
	AppealID string `json:"appealID" binding:"required"`
	Action   string `json:"action" binding:"required,oneof=approve reject reviewing"`
	Note     string `json:"note"`
	// Action=approve 必填，要解封的用户ID（可与 account 不同）
	UnblockUserID string `json:"unblockUserID"`
}

func (o *GovernanceApi) HandleAppeal(c *gin.Context) {
	var req HandleAppealReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	adminID, err := mctx.CheckAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	a, err := o.appeal.Take(c, req.AppealID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	status := req.Action
	switch req.Action {
	case "approve":
		uid := req.UnblockUserID
		if uid == "" {
			uid = a.UserID
		}
		if uid == "" {
			uid = a.Account
		}
		_, uerr := o.adminClient.UnblockUser(c, &admin.UnblockUserReq{UserIDs: []string{uid}})
		if uerr != nil {
			apiresp.GinError(c, uerr)
			return
		}
		status = "approved"
	case "reject":
		status = "rejected"
	case "reviewing":
		status = "reviewing"
	}
	now := time.Now()
	upd := map[string]any{
		"status":      status,
		"handler_id":  adminID,
		"handle_note": req.Note,
	}
	if status == "approved" || status == "rejected" {
		upd["handle_time"] = now
	}
	if err := o.appeal.Update(c, req.AppealID, upd); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, gin.H{})
}

// ─── Appeal (public, no auth) ────────────────────────────────────────────

func (o *GovernanceApi) RequestAppealCaptcha(c *gin.Context) {
	ch, err := o.captchaSvc.Generate(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, ch)
}

type CreateAppealReq struct {
	Account       string `json:"account" binding:"required"`
	Reason        string `json:"reason" binding:"required"`
	Description   string `json:"description"`
	Contact       string `json:"contact"`
	CaptchaID     string `json:"captchaID" binding:"required"`
	CaptchaAnswer string `json:"captchaAnswer" binding:"required"`
}

type CreateAppealResp struct {
	AppealID    string `json:"appealID"`
	AppealToken string `json:"appealToken"`
}

func (o *GovernanceApi) CreateAppeal(c *gin.Context) {
	var req CreateAppealReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	req.Account = strings.TrimSpace(req.Account)
	if !o.captchaSvc.Verify(c, req.CaptchaID, strings.TrimSpace(req.CaptchaAnswer)) {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("captcha invalid"))
		return
	}
	clientIP := c.ClientIP()
	// 限频：1h 内同 account 或同 IP 不超过 3 次
	count, err := o.appeal.CountRecent(c, req.Account, clientIP, time.Now().Add(-time.Hour))
	if err == nil && count >= 3 {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("too many appeals, try later"))
		return
	}
	a := &admindb.Appeal{
		AppealID:    uuid.NewString(),
		Account:     req.Account,
		UserID:      req.Account, // v1: 假设 account 即 userID；若是手机号则 admin 处理时再确认
		Reason:      req.Reason,
		Description: req.Description,
		Contact:     req.Contact,
		Evidence:    []string{},
		Status:      "pending",
		ClientIP:    clientIP,
		CreateTime:  time.Now(),
	}
	if err := o.appeal.Create(c, []*admindb.Appeal{a}); err != nil {
		apiresp.GinError(c, err)
		return
	}
	token, err := o.AppealTokens.Issue(c, a.AppealID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &CreateAppealResp{AppealID: a.AppealID, AppealToken: token})
}

type AppealUploadResp struct {
	URL string `json:"url"`
}

func (o *GovernanceApi) UploadAppealEvidence(c *gin.Context) {
	appealID := chatmw.GetAppealID(c)
	if appealID == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("missing appeal context"))
		return
	}
	a, err := o.appeal.Take(c, appealID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if len(a.Evidence) >= storage.DefaultMaxFiles {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("evidence count limit reached"))
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	url, err := o.storage.Save(appealID, fh)
	if err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if err := o.appeal.AddEvidence(c, appealID, url); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &AppealUploadResp{URL: url})
}
