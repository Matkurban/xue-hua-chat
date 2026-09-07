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

// sendFavoriteNotification sends a business notification for favorite events.
func (o *Api) sendFavoriteNotification(c *gin.Context, imToken, key string, recvUserID string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.ZWarn(c, "marshal favorite notification payload", err, "key", key)
		return
	}
	ctx := mctx.WithApiToken(c, imToken)
	if err := o.imApiCaller.SendBusinessNotification(ctx, key, string(data), o.ImUserID, recvUserID); err != nil {
		log.ZWarn(c, "send favorite notification failed", err, "key", key, "recvUserID", recvUserID)
	}
}

func (o *Api) AddFavorite(c *gin.Context) {
	var req imapi.AddFavoriteReq
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
	resp, err := o.imApiCaller.AddFavorite(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)

	// Notify the user for multi-device sync
	go o.sendFavoriteNotification(c.Copy(), imToken, imapi.NotifyKeyFavoriteAdded, userID, resp)
}

func (o *Api) RemoveFavorite(c *gin.Context) {
	var req imapi.RemoveFavoriteReq
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
	if err := o.imApiCaller.RemoveFavorite(mctx.WithApiToken(c, imToken), &req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, nil)

	go o.sendFavoriteNotification(c.Copy(), imToken, imapi.NotifyKeyFavoriteRemoved, userID, map[string]string{
		"targetType": req.TargetType,
		"targetID":   req.TargetID,
	})
}

func (o *Api) ListFavorites(c *gin.Context) {
	var req imapi.ListFavoritesReq
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
	resp, err := o.imApiCaller.ListFavorites(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) UpdateFavorite(c *gin.Context) {
	var req imapi.UpdateFavoriteReq
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
	resp, err := o.imApiCaller.UpdateFavorite(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)

	go o.sendFavoriteNotification(c.Copy(), imToken, imapi.NotifyKeyFavoriteUpdated, userID, resp)
}
