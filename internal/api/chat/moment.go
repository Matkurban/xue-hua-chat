package chat

import (
	"encoding/json"

	"chat/pkg/common/apiresp"
	"chat/pkg/common/errs"
	"chat/pkg/common/imapi"
	"chat/pkg/common/log"
	"chat/pkg/common/mctx"
	"github.com/gin-gonic/gin"
)

// sendMomentNotification sends a business notification via xue-hua-server WS push.
// Errors are logged but not propagated — notification failure must not block the API response.
func (o *Api) sendMomentNotification(c *gin.Context, imToken, key string, recvUserID string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.ZWarn(c, "marshal moment notification payload", err, "key", key)
		return
	}
	ctx := mctx.WithApiToken(c, imToken)
	if err := o.imApiCaller.SendBusinessNotification(ctx, key, string(data), o.ImUserID, recvUserID); err != nil {
		log.ZWarn(c, "send moment notification failed", err, "key", key, "recvUserID", recvUserID)
	}
}

func (o *Api) notifyMomentRecipients(c *gin.Context, imToken, key string, recipients []string, payload any) {
	if len(recipients) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(recipients))
	for _, uid := range recipients {
		if uid == "" {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		o.sendMomentNotification(c, imToken, key, uid, payload)
	}
}

func (o *Api) CreateMoment(c *gin.Context) {
	var req imapi.CreateMomentReq
	if err := c.BindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WithDetail(err.Error()).Wrap())
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.UserID = userID
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.imApiCaller.CreateMoment(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	recipients := resp.VisibleUserIDs
	resp.VisibleUserIDs = nil
	apiresp.GinSuccess(c, resp)

	// Notify only users who are permitted to see this moment.
	go func() {
		ctx := c.Copy()
		payload := map[string]any{
			"moment":  resp.Moment,
			"syncSeq": resp.SyncSeq,
		}
		o.notifyMomentRecipients(ctx, imToken, imapi.NotifyKeyMomentCreated, recipients, payload)
	}()
}

func (o *Api) DeleteMoment(c *gin.Context) {
	var req imapi.DeleteMomentReq
	if err := c.BindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WithDetail(err.Error()).Wrap())
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.UserID = userID
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.imApiCaller.DeleteMoment(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	var recipients []string
	var syncSeq int64
	if resp != nil {
		recipients = resp.VisibleUserIDs
		syncSeq = resp.SyncSeq
	}
	apiresp.GinSuccess(c, map[string]any{"syncSeq": syncSeq})

	go func() {
		ctx := c.Copy()
		payload := map[string]any{
			"momentID": req.MomentID,
			"syncSeq":  syncSeq,
		}
		o.notifyMomentRecipients(ctx, imToken, imapi.NotifyKeyMomentDeleted, recipients, payload)
	}()
}

func (o *Api) ListMoments(c *gin.Context) {
	var req imapi.ListMomentsReq
	if err := c.BindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WithDetail(err.Error()).Wrap())
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.UserID = userID
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.imApiCaller.ListMoments(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) ListTimeline(c *gin.Context) {
	var req imapi.ListTimelineReq
	if err := c.BindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WithDetail(err.Error()).Wrap())
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.UserID = userID
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.imApiCaller.ListTimeline(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) SyncMoments(c *gin.Context) {
	var req imapi.SyncMomentsReq
	if err := c.BindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WithDetail(err.Error()).Wrap())
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.UserID = userID
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.imApiCaller.SyncMoments(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) CommentMoment(c *gin.Context) {
	var req imapi.CommentMomentReq
	if err := c.BindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WithDetail(err.Error()).Wrap())
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.UserID = userID
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.imApiCaller.CommentMoment(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	recipients := resp.VisibleUserIDs
	syncSeq := resp.SyncSeq
	resp.VisibleUserIDs = nil
	apiresp.GinSuccess(c, resp)

	go func() {
		ctx := c.Copy()
		if len(recipients) == 0 && req.OwnerUserID != "" {
			recipients = []string{req.OwnerUserID, userID}
		}
		payload := map[string]any{
			"momentID": req.MomentID,
			"comment":  resp.Comment,
			"syncSeq":  syncSeq,
		}
		o.notifyMomentRecipients(ctx, imToken, imapi.NotifyKeyMomentCommented, recipients, payload)
	}()
}

func (o *Api) DeleteComment(c *gin.Context) {
	var req imapi.DeleteCommentReq
	if err := c.BindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WithDetail(err.Error()).Wrap())
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.UserID = userID
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.imApiCaller.DeleteComment(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	var recipients []string
	var syncSeq int64
	var momentID string
	if resp != nil {
		recipients = resp.VisibleUserIDs
		syncSeq = resp.SyncSeq
		momentID = resp.MomentID
	}
	apiresp.GinSuccess(c, map[string]any{
		"momentID": momentID,
		"syncSeq":  syncSeq,
	})

	go func() {
		ctx := c.Copy()
		if len(recipients) == 0 {
			recipients = []string{userID}
		}
		payload := map[string]any{
			"momentID":  momentID,
			"commentID": req.CommentID,
			"syncSeq":   syncSeq,
		}
		o.notifyMomentRecipients(ctx, imToken, imapi.NotifyKeyMomentCommentDeleted, recipients, payload)
	}()
}

func (o *Api) LikeMoment(c *gin.Context) {
	var req imapi.LikeMomentReq
	if err := c.BindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WithDetail(err.Error()).Wrap())
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.UserID = userID
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.imApiCaller.LikeMoment(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	var recipients []string
	var syncSeq int64
	var like any
	if resp != nil {
		recipients = resp.VisibleUserIDs
		syncSeq = resp.SyncSeq
		like = resp.Like
	}
	if like == nil {
		like = map[string]string{
			"momentID": req.MomentID,
			"userID":   userID,
		}
	}
	apiresp.GinSuccess(c, map[string]any{
		"syncSeq": syncSeq,
		"like":    like,
	})

	go func() {
		ctx := c.Copy()
		if len(recipients) == 0 && req.OwnerUserID != "" {
			recipients = []string{req.OwnerUserID, userID}
		}
		payload := map[string]any{
			"momentID": req.MomentID,
			"like":     like,
			"syncSeq":  syncSeq,
		}
		o.notifyMomentRecipients(ctx, imToken, imapi.NotifyKeyMomentLiked, recipients, payload)
	}()
}

func (o *Api) UnlikeMoment(c *gin.Context) {
	var req imapi.UnlikeMomentReq
	if err := c.BindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WithDetail(err.Error()).Wrap())
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.UserID = userID
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.imApiCaller.UnlikeMoment(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	var recipients []string
	var syncSeq int64
	if resp != nil {
		recipients = resp.VisibleUserIDs
		syncSeq = resp.SyncSeq
	}
	apiresp.GinSuccess(c, map[string]any{"syncSeq": syncSeq})

	go func() {
		ctx := c.Copy()
		if len(recipients) == 0 && req.OwnerUserID != "" {
			recipients = []string{req.OwnerUserID, userID}
		}
		payload := map[string]any{
			"momentID": req.MomentID,
			"userID":   userID,
			"syncSeq":  syncSeq,
		}
		o.notifyMomentRecipients(ctx, imToken, imapi.NotifyKeyMomentUnliked, recipients, payload)
	}()
}
