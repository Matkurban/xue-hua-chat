package admin

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"chat/pkg/common/db/table/admin"
	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"gitee.com/matkurban/openimsdk-tools/log"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/openimsdk/protocol/constant"
	"github.com/openimsdk/protocol/sdkws"
)

const (
	oaNotificationSessionType = constant.NotificationChatType
	systemNotifRevokeBatch    = 500
	systemNotifMaxRecv        = 5000
	persistRetryAttempts      = 3
	persistRetryBase          = 100 * time.Millisecond
)

type SystemNotificationApi struct {
	store    admin.SystemNotificationInterface
	targets  admin.SystemNotificationTargetInterface
	im       imapi.CallerInterface
	imUserID string
}

func NewSystemNotificationApi(
	store admin.SystemNotificationInterface,
	targets admin.SystemNotificationTargetInterface,
	im imapi.CallerInterface,
	imUserID string,
) *SystemNotificationApi {
	return &SystemNotificationApi{store: store, targets: targets, im: im, imUserID: imUserID}
}

type systemNotificationSendReq struct {
	NotificationName string   `json:"notificationName"`
	Text             string   `json:"text"`
	ImageURL         string   `json:"imageUrl"`
	ExternalURL      string   `json:"externalUrl"`
	SenderNickname   string   `json:"senderNickname"`
	SenderFaceURL    string   `json:"senderFaceURL"`
	RecvIDs          []string `json:"recvIDs"`
	NotificationType int32    `json:"notificationType"`
	IsSendAll        bool     `json:"isSendAll"`
	NotOfflinePush   bool     `json:"notOfflinePush"`
}

type systemNotificationPageReq struct {
	PageNumber int32 `json:"pageNumber"`
	ShowNumber int32 `json:"showNumber"`
}

func (p systemNotificationPageReq) GetPageNumber() int32 {
	if p.PageNumber <= 0 {
		return 1
	}
	return p.PageNumber
}

func (p systemNotificationPageReq) GetShowNumber() int32 {
	if p.ShowNumber <= 0 || p.ShowNumber > 200 {
		return 20
	}
	return p.ShowNumber
}

type systemNotificationRevokeReq struct {
	NotificationID string `json:"notificationID"`
}

type systemNotificationItem struct {
	NotificationID   string `json:"notificationID"`
	SendID           string `json:"sendID"`
	SenderNickname   string `json:"senderNickname"`
	Title            string `json:"title"`
	Text             string `json:"text"`
	ImageURL         string `json:"imageUrl"`
	ExternalURL      string `json:"externalUrl"`
	Status           string `json:"status"`
	OperatorAdminID  string `json:"operatorAdminID"`
	RevokeBy         string `json:"revokeBy,omitempty"`
	CreateTime       int64  `json:"createTime"`
	RevokeTime       int64  `json:"revokeTime,omitempty"`
	NotificationType int32  `json:"notificationType"`
	SuccessCount     int32  `json:"successCount"`
	FailedCount      int32  `json:"failedCount"`
	RevokeSuccess    int32  `json:"revokeSuccess"`
	RevokeFailed     int32  `json:"revokeFailed"`
	IsSendAll        bool   `json:"isSendAll"`
}

func toSystemNotificationItem(n *admin.SystemNotification) systemNotificationItem {
	item := systemNotificationItem{
		NotificationID:   n.NotificationID,
		SendID:           n.SendID,
		SenderNickname:   n.SenderNickname,
		NotificationType: n.NotificationType,
		Title:            n.Title,
		Text:             n.Text,
		ImageURL:         n.ImageURL,
		ExternalURL:      n.ExternalURL,
		IsSendAll:        n.IsSendAll,
		SuccessCount:     n.SuccessCount,
		FailedCount:      n.FailedCount,
		Status:           n.Status,
		OperatorAdminID:  n.OperatorAdminID,
		CreateTime:       n.CreateTime.UnixMilli(),
		RevokeBy:         n.RevokeBy,
		RevokeSuccess:    n.RevokeSuccess,
		RevokeFailed:     n.RevokeFailed,
	}
	if !n.RevokeTime.IsZero() {
		item.RevokeTime = n.RevokeTime.UnixMilli()
	}
	return item
}

func (o *SystemNotificationApi) Send(c *gin.Context) {
	if o == nil || o.store == nil || o.targets == nil || o.im == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("system notification unavailable"))
		return
	}
	var req systemNotificationSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	title := strings.TrimSpace(req.NotificationName)
	body := strings.TrimSpace(req.Text)
	if title == "" || body == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("title and text are required"))
		return
	}
	if req.NotificationType != 1 && req.NotificationType != 2 {
		req.NotificationType = 1
	}
	if !req.IsSendAll && len(req.RecvIDs) == 0 {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("recvIDs is required when not sending to all"))
		return
	}
	if !req.IsSendAll && len(req.RecvIDs) > systemNotifMaxRecv {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("too many recipients"))
		return
	}
	sendID := o.imUserID
	if sendID == "" {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("im admin user id missing"))
		return
	}
	operatorID := mctx.GetOpUserID(c)
	imToken, err := o.im.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiCtx, cancel := context.WithTimeout(mctx.WithApiToken(c, imToken), 10*time.Minute)
	defer cancel()

	picture := strings.TrimSpace(req.ImageURL)
	link := strings.TrimSpace(req.ExternalURL)
	hasPicture := picture != ""
	content := map[string]any{
		"notificationName":    title,
		"notificationFaceURL": req.SenderFaceURL,
		"notificationType":    req.NotificationType,
		"text":                body,
		"externalUrl":         link,
		"mixType":             0,
		"ex":                  "",
	}
	if hasPicture {
		content["mixType"] = 1
		content["pictureElem"] = buildOAPictureElem(picture)
	}
	nickname := strings.TrimSpace(req.SenderNickname)
	if nickname == "" {
		nickname = "系统通知"
	}
	recvIDs := req.RecvIDs
	if req.IsSendAll {
		// IM BindJSON marks recvIDs as required; ignored when isSendAll is true.
		recvIDs = []string{sendID}
	}
	var push *sdkws.OfflinePushInfo
	if !req.NotOfflinePush {
		push = &sdkws.OfflinePushInfo{
			Title:         title,
			Desc:          pushDesc(body, 80),
			IOSBadgeCount: true,
		}
	}
	imResp, err := o.im.BatchSendMsg(apiCtx, &imapi.BatchSendMsgReq{
		SendID:           sendID,
		SenderNickname:   nickname,
		SenderFaceURL:    req.SenderFaceURL,
		SenderPlatformID: 0,
		ContentType:      constant.OANotification,
		SessionType:      oaNotificationSessionType,
		Content:          content,
		IsSendAll:        req.IsSendAll,
		RecvIDs:          recvIDs,
		NotOfflinePush:   req.NotOfflinePush,
		OfflinePushInfo:  push,
	})
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if imResp == nil {
		imResp = &imapi.BatchSendMsgResp{}
	}

	now := time.Now()
	n := &admin.SystemNotification{
		NotificationID:   uuid.NewString(),
		SendID:           sendID,
		SenderNickname:   nickname,
		NotificationType: req.NotificationType,
		Title:            title,
		Text:             body,
		ImageURL:         picture,
		ExternalURL:      link,
		IsSendAll:        req.IsSendAll,
		SuccessCount:     int32(len(imResp.Results)),
		FailedCount:      int32(len(imResp.FailedUserIDs)),
		Status:           admin.SystemNotificationStatusSent,
		OperatorAdminID:  operatorID,
		CreateTime:       now,
	}
	if err := persistWithRetry(apiCtx, func() error { return o.store.Create(apiCtx, n) }); err != nil {
		log.ZError(apiCtx, "persist system notification record failed", err, "notificationID", n.NotificationID)
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("notification delivered but failed to save send record; do not resend"))
		return
	}
	targets := make([]*admin.SystemNotificationTarget, 0, len(imResp.Results))
	for _, r := range imResp.Results {
		if r == nil || r.RecvID == "" || r.ClientMsgID == "" {
			continue
		}
		targets = append(targets, &admin.SystemNotificationTarget{
			NotificationID: n.NotificationID,
			RecvID:         r.RecvID,
			ClientMsgID:    r.ClientMsgID,
			ServerMsgID:    r.ServerMsgID,
			SendTime:       r.SendTime,
			Status:         admin.SystemNotificationTargetSent,
		})
	}
	if err := persistWithRetry(apiCtx, func() error { return o.targets.CreateMany(apiCtx, targets) }); err != nil {
		log.ZError(apiCtx, "persist system notification targets failed", err, "notificationID", n.NotificationID, "count", len(targets))
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("notification delivered but failed to save recipients; do not resend, revoke may be unavailable"))
		return
	}
	apiresp.GinSuccess(c, map[string]any{
		"notification":  toSystemNotificationItem(n),
		"resultsCount":  len(imResp.Results),
		"failedUserIDs": imResp.FailedUserIDs,
	})
}

func (o *SystemNotificationApi) List(c *gin.Context) {
	if o == nil || o.store == nil {
		apiresp.GinSuccess(c, map[string]any{"list": []systemNotificationItem{}, "total": 0})
		return
	}
	var req systemNotificationPageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	total, rows, err := o.store.Search(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	list := make([]systemNotificationItem, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		list = append(list, toSystemNotificationItem(row))
	}
	apiresp.GinSuccess(c, map[string]any{"list": list, "total": total})
}

func (o *SystemNotificationApi) Revoke(c *gin.Context) {
	if o == nil || o.store == nil || o.targets == nil || o.im == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("system notification unavailable"))
		return
	}
	var req systemNotificationRevokeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if strings.TrimSpace(req.NotificationID) == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("notificationID is required"))
		return
	}
	n, err := o.store.Take(c, req.NotificationID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if n.Status == admin.SystemNotificationStatusRevoked {
		apiresp.GinSuccess(c, toSystemNotificationItem(n))
		return
	}
	if n.SuccessCount == 0 {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("no delivered messages to revoke"))
		return
	}
	totalTargets, err := o.targets.CountByNotification(c, n.NotificationID, "")
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if totalTargets == 0 {
		apiresp.GinError(c, errs.ErrRecordNotFound.WrapMsg("no recipient records to revoke"))
		return
	}
	imToken, err := o.im.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	operatorID := mctx.GetOpUserID(c)
	_ = o.store.Update(c, n.NotificationID, map[string]any{
		"status": admin.SystemNotificationStatusRevoking,
	})
	apiCtx, cancel := context.WithTimeout(mctx.WithApiToken(c, imToken), 10*time.Minute)
	defer cancel()

	var skip int64
	for {
		batch, err := o.targets.ListByNotification(apiCtx, n.NotificationID, skip, systemNotifRevokeBatch)
		if err != nil {
			apiresp.GinError(c, err)
			return
		}
		if len(batch) == 0 {
			break
		}
		skip += int64(len(batch))
		items, pendingIDs := pendingRevokeItems(batch)
		if len(items) == 0 {
			continue
		}
		imResp, err := o.im.BatchRevokeMsg(apiCtx, &imapi.BatchRevokeMsgReq{
			SendID:      n.SendID,
			SessionType: oaNotificationSessionType,
			Items:       items,
		})
		if err != nil {
			_ = o.targets.MarkStatus(apiCtx, n.NotificationID, pendingIDs, admin.SystemNotificationTargetFailed)
			continue
		}
		failedSet := map[string]struct{}{}
		if imResp != nil {
			for _, id := range imResp.FailedUserIDs {
				failedSet[id] = struct{}{}
			}
		}
		successIDs := make([]string, 0, len(pendingIDs))
		failIDs := make([]string, 0)
		for _, id := range pendingIDs {
			if _, ok := failedSet[id]; ok {
				failIDs = append(failIDs, id)
			} else {
				successIDs = append(successIDs, id)
			}
		}
		_ = o.targets.MarkStatus(apiCtx, n.NotificationID, successIDs, admin.SystemNotificationTargetRevoked)
		_ = o.targets.MarkStatus(apiCtx, n.NotificationID, failIDs, admin.SystemNotificationTargetFailed)
	}

	revokeSuccess64, _ := o.targets.CountByNotification(apiCtx, n.NotificationID, admin.SystemNotificationTargetRevoked)
	revokeFailed64, _ := o.targets.CountByNotification(apiCtx, n.NotificationID, admin.SystemNotificationTargetFailed)
	revokeSuccess := int32(revokeSuccess64)
	revokeFailed := int32(revokeFailed64)
	status := admin.SystemNotificationStatusRevoked
	if revokeFailed > 0 {
		status = admin.SystemNotificationStatusPartial
	}
	now := time.Now()
	_ = o.store.Update(apiCtx, n.NotificationID, map[string]any{
		"status":         status,
		"revoke_time":    now,
		"revoke_by":      operatorID,
		"revoke_success": revokeSuccess,
		"revoke_failed":  revokeFailed,
	})
	n.Status = status
	n.RevokeTime = now
	n.RevokeBy = operatorID
	n.RevokeSuccess = revokeSuccess
	n.RevokeFailed = revokeFailed
	apiresp.GinSuccess(c, toSystemNotificationItem(n))
}

func persistWithRetry(ctx context.Context, fn func() error) error {
	var last error
	delay := persistRetryBase
	for i := 0; i < persistRetryAttempts; i++ {
		last = fn()
		if last == nil {
			return nil
		}
		if i == persistRetryAttempts-1 {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			if err := ctx.Err(); err != nil {
				return err
			}
			return last
		case <-timer.C:
		}
		delay *= 2
	}
	return last
}

func pendingRevokeItems(batch []*admin.SystemNotificationTarget) ([]imapi.BatchRevokeMsgItem, []string) {
	items := make([]imapi.BatchRevokeMsgItem, 0, len(batch))
	pendingIDs := make([]string, 0, len(batch))
	for _, t := range batch {
		if t == nil || t.Status == admin.SystemNotificationTargetRevoked {
			continue
		}
		items = append(items, imapi.BatchRevokeMsgItem{RecvID: t.RecvID, ClientMsgID: t.ClientMsgID})
		pendingIDs = append(pendingIDs, t.RecvID)
	}
	return items, pendingIDs
}

func pushDesc(content string, max int) string {
	s := strings.TrimSpace(content)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}

func inferImageType(url string) string {
	path := strings.ToLower(strings.Split(url, "?")[0])
	switch {
	case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(path, ".gif"):
		return "image/gif"
	case strings.HasSuffix(path, ".webp"):
		return "image/webp"
	case strings.HasSuffix(path, ".bmp"):
		return "image/bmp"
	default:
		return "image/png"
	}
}

func buildOAPictureElem(url string) map[string]any {
	info := map[string]any{
		"type":   inferImageType(url),
		"size":   0,
		"width":  720,
		"height": 1280,
		"url":    url,
	}
	return map[string]any{
		"sourcePath":      "",
		"sourcePicture":   info,
		"bigPicture":      info,
		"snapshotPicture": info,
	}
}
