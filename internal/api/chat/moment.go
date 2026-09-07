package chat

import (
	"encoding/json"

	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"gitee.com/matkurban/openimsdk-tools/log"
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
	apiresp.GinSuccess(c, resp)

	// Notify all friends about the new moment
	go func() {
		ctx := c.Copy()
		friendIDs, err := o.imApiCaller.FriendUserIDs(mctx.WithApiToken(ctx, imToken), userID)
		if err != nil {
			log.ZWarn(ctx, "get friend ids for moment notification", err, "userID", userID)
			return
		}
		for _, fid := range friendIDs {
			o.sendMomentNotification(ctx, imToken, imapi.NotifyKeyMomentCreated, fid, resp)
		}
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
	if err := o.imApiCaller.DeleteMoment(mctx.WithApiToken(c, imToken), &req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, nil)

	// Notify all friends about the deletion
	go func() {
		ctx := c.Copy()
		payload := map[string]string{"momentID": req.MomentID}
		friendIDs, err := o.imApiCaller.FriendUserIDs(mctx.WithApiToken(ctx, imToken), userID)
		if err != nil {
			log.ZWarn(ctx, "get friend ids for moment delete notification", err, "userID", userID)
			return
		}
		for _, fid := range friendIDs {
			o.sendMomentNotification(ctx, imToken, imapi.NotifyKeyMomentDeleted, fid, payload)
		}
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
	apiresp.GinSuccess(c, resp)

	// Notify the moment owner (if not self)
	go func() {
		if req.OwnerUserID != "" && req.OwnerUserID != userID {
			ctx := c.Copy()
			payload := map[string]any{
				"momentID": req.MomentID,
				"comment":  resp.Comment,
			}
			o.sendMomentNotification(ctx, imToken, imapi.NotifyKeyMomentCommented, req.OwnerUserID, payload)
		}
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
	if err := o.imApiCaller.DeleteComment(mctx.WithApiToken(c, imToken), &req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, nil)

	go o.sendMomentNotification(c.Copy(), imToken, imapi.NotifyKeyMomentCommentDeleted, userID, map[string]string{"commentID": req.CommentID})
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
	if err := o.imApiCaller.LikeMoment(mctx.WithApiToken(c, imToken), &req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, nil)

	// Notify the moment owner (if not self)
	go func() {
		if req.OwnerUserID != "" && req.OwnerUserID != userID {
			ctx := c.Copy()
			payload := map[string]any{
				"momentID": req.MomentID,
				"like": map[string]string{
					"momentID": req.MomentID,
					"userID":   userID,
				},
			}
			o.sendMomentNotification(ctx, imToken, imapi.NotifyKeyMomentLiked, req.OwnerUserID, payload)
		}
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
	if err := o.imApiCaller.UnlikeMoment(mctx.WithApiToken(c, imToken), &req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, nil)

	// Notify the moment owner (if not self)
	go func() {
		if req.OwnerUserID != "" && req.OwnerUserID != userID {
			ctx := c.Copy()
			payload := map[string]string{
				"momentID": req.MomentID,
				"userID":   userID,
			}
			o.sendMomentNotification(ctx, imToken, imapi.NotifyKeyMomentUnliked, req.OwnerUserID, payload)
		}
	}()
}
