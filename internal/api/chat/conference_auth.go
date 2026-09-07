package chat

import (
	"chat/pkg/common/constant"
	"chat/pkg/common/mctx"
	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"github.com/gin-gonic/gin"
)

// bindConferenceOpUser forces the request actor userID to the authenticated operator.
func bindConferenceOpUser(c *gin.Context, reqUserID *string) bool {
	op := ""
	if v, ok := c.Get(constant.RpcOpUserID); ok {
		op, _ = v.(string)
	}
	if op == "" {
		op = mctx.GetOpUserID(c)
	}
	if op == "" {
		apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("missing operator"))
		return false
	}
	if *reqUserID != "" && *reqUserID != op {
		apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("userID must match login user"))
		return false
	}
	*reqUserID = op
	return true
}
