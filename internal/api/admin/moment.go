package admin

import (
	"context"
	"errors"
	"io"

	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"chat/pkg/common/apiresp"
	"chat/pkg/common/errs"
	"github.com/gin-gonic/gin"
)

// momentPassThrough binds the JSON body to Req (unless Req is nil-body), forwards it to
// xue-hua-server with the IM admin token and writes the response unchanged.
// Authorization (chat admin token) is enforced by mw.CheckAdmin on the route group.
func momentPassThrough[Req, Resp any](o *Api, c *gin.Context, call func(ctx context.Context, req *Req) (*Resp, error)) {
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) { // empty body allowed (e.g. stats)
		apiresp.GinError(c, errs.ErrArgs.WithDetail(err.Error()).Wrap())
		return
	}
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := call(mctx.WithApiToken(c, imToken), &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) MomentAdminList(c *gin.Context) {
	momentPassThrough(o, c, o.imApiCaller.AdminListMoments)
}

func (o *Api) MomentAdminDetail(c *gin.Context) {
	momentPassThrough(o, c, o.imApiCaller.AdminGetMoment)
}

func (o *Api) MomentAdminDelete(c *gin.Context) {
	momentPassThrough(o, c, o.imApiCaller.AdminDeleteMoments)
}

func (o *Api) MomentAdminCommentList(c *gin.Context) {
	momentPassThrough(o, c, o.imApiCaller.AdminListMomentComments)
}

func (o *Api) MomentAdminDeleteComment(c *gin.Context) {
	momentPassThrough(o, c, func(ctx context.Context, req *imapi.AdminDeleteCommentReq) (*struct{}, error) {
		return nil, o.imApiCaller.AdminDeleteMomentComment(ctx, req)
	})
}

func (o *Api) MomentAdminLikeList(c *gin.Context) {
	momentPassThrough(o, c, o.imApiCaller.AdminListMomentLikes)
}

func (o *Api) MomentAdminBan(c *gin.Context) {
	momentPassThrough(o, c, o.imApiCaller.AdminBanMoments)
}

func (o *Api) MomentAdminRestore(c *gin.Context) {
	momentPassThrough(o, c, o.imApiCaller.AdminRestoreMoments)
}

func (o *Api) MomentAdminStats(c *gin.Context) {
	momentPassThrough(o, c, func(ctx context.Context, _ *imapi.AdminMomentStatsReq) (*imapi.AdminMomentStatsResp, error) {
		return o.imApiCaller.AdminMomentStats(ctx)
	})
}
