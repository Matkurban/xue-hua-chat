package chat

import (
	"crypto/subtle"
	"time"

	"chat/pkg/common/db/database"
	"chat/pkg/common/mctx"
	"chat/pkg/common/apiresp"
	"chat/pkg/common/errs"
	"github.com/gin-gonic/gin"
)

// PaymentPasswordApi 支付密码功能 HTTP 处理器
type PaymentPasswordApi struct {
	db database.ChatDatabaseInterface
}

// NewPaymentPasswordApi 创建 PaymentPasswordApi 实例
func NewPaymentPasswordApi(db database.ChatDatabaseInterface) *PaymentPasswordApi {
	return &PaymentPasswordApi{db: db}
}

// ─── 请求 / 响应结构 ─────────────────────────────────────────────────────────

type SetPaymentPasswordReq struct {
	PaymentPassword string `json:"paymentPassword" binding:"required,len=6,numeric"`
	LoginPassword   string `json:"loginPassword" binding:"required"`
}

type ChangePaymentPasswordReq struct {
	CurrentPaymentPassword string `json:"currentPaymentPassword" binding:"required,len=6,numeric"`
	NewPaymentPassword     string `json:"newPaymentPassword" binding:"required,len=6,numeric"`
}

type VerifyPaymentPasswordReq struct {
	PaymentPassword string `json:"paymentPassword" binding:"required,len=6,numeric"`
}

type CheckPaymentPasswordSetResp struct {
	IsSet bool `json:"isSet"`
}

type ResetPaymentPasswordReq struct {
	AreaCode           string `json:"areaCode"`
	PhoneNumber        string `json:"phoneNumber"`
	Email              string `json:"email"`
	VerifyCode         string `json:"verifyCode" binding:"required"`
	NewPaymentPassword string `json:"newPaymentPassword" binding:"required,len=6,numeric"`
}

// ─── 处理器 ─────────────────────────────────────────────────────────────────

// SetPaymentPassword POST /user/payment_password/set
// 首次设置支付密码，需验证登录密码。
func (o *PaymentPasswordApi) SetPaymentPassword(c *gin.Context) {
	var req SetPaymentPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ctx := c.Request.Context()

	// 获取用户账号信息
	account, err := o.db.GetUser(ctx, userID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	// 验证登录密码（常量时间比较，防止计时侧信道攻击）
	if subtle.ConstantTimeCompare([]byte(account.Password), []byte(req.LoginPassword)) != 1 {
		apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("login password is wrong"))
		return
	}

	// 检查是否已设置支付密码
	if account.PaymentPassword != "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("payment password already set, use change instead"))
		return
	}

	// 设置支付密码
	if err := o.db.UpdatePaymentPassword(ctx, userID, req.PaymentPassword); err != nil {
		apiresp.GinError(c, err)
		return
	}

	apiresp.GinSuccess(c, nil)
}

// ChangePaymentPassword POST /user/payment_password/change
// 修改支付密码，需验证当前支付密码。
func (o *PaymentPasswordApi) ChangePaymentPassword(c *gin.Context) {
	var req ChangePaymentPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ctx := c.Request.Context()

	if req.CurrentPaymentPassword == req.NewPaymentPassword {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("new payment password must differ from current"))
		return
	}

	// 获取用户账号信息
	account, err := o.db.GetUser(ctx, userID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	// 必须已设置支付密码
	if account.PaymentPassword == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("payment password not set yet"))
		return
	}

	// 验证当前支付密码（常量时间比较）
	if subtle.ConstantTimeCompare([]byte(account.PaymentPassword), []byte(req.CurrentPaymentPassword)) != 1 {
		apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("current payment password is wrong"))
		return
	}

	// 更新支付密码
	if err := o.db.UpdatePaymentPassword(ctx, userID, req.NewPaymentPassword); err != nil {
		apiresp.GinError(c, err)
		return
	}

	apiresp.GinSuccess(c, nil)
}

// VerifyPaymentPassword POST /user/payment_password/verify
// 验证支付密码（其他业务调用前验证）。
func (o *PaymentPasswordApi) VerifyPaymentPassword(c *gin.Context) {
	var req VerifyPaymentPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ctx := c.Request.Context()

	account, err := o.db.GetUser(ctx, userID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	if account.PaymentPassword == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("payment password not set"))
		return
	}

	if subtle.ConstantTimeCompare([]byte(account.PaymentPassword), []byte(req.PaymentPassword)) != 1 {
		apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("payment password is wrong"))
		return
	}

	apiresp.GinSuccess(c, nil)
}

// CheckPaymentPasswordSet POST /user/payment_password/check
// 检查用户是否已设置支付密码。
func (o *PaymentPasswordApi) CheckPaymentPasswordSet(c *gin.Context) {
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ctx := c.Request.Context()

	account, err := o.db.GetUser(ctx, userID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	apiresp.GinSuccess(c, &CheckPaymentPasswordSetResp{
		IsSet: account.PaymentPassword != "",
	})
}

// ResetPaymentPassword POST /user/payment_password/reset
// 忘记支付密码，通过手机号或邮箱验证码重置。
func (o *PaymentPasswordApi) ResetPaymentPassword(c *gin.Context) {
	var req ResetPaymentPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ctx := c.Request.Context()

	// 获取用户属性，确认手机号或邮箱属于当前用户
	attr, err := o.db.TakeAttributeByUserID(ctx, userID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	// 确定验证码对应的 account 标识
	var codeAccount string
	if req.Email != "" {
		if attr.Email != req.Email {
			apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("email does not match current user"))
			return
		}
		codeAccount = req.Email
	} else if req.PhoneNumber != "" {
		if attr.PhoneNumber != req.PhoneNumber || attr.AreaCode != req.AreaCode {
			apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("phone number does not match current user"))
			return
		}
		codeAccount = req.AreaCode + req.PhoneNumber
	} else {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("email or phoneNumber is required"))
		return
	}

	// 验证验证码
	last, err := o.db.TakeLastVerifyCode(ctx, codeAccount)
	if err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("verify code not found"))
		return
	}
	if last.Used {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("verify code already used"))
		return
	}
	if last.CreateTime.Add(time.Duration(last.Duration) * time.Minute).Before(time.Now()) {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("verify code expired"))
		return
	}
	if last.Count >= 5 {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("verify code max attempts exceeded"))
		return
	}
	if last.Code != req.VerifyCode {
		_ = o.db.UpdateVerifyCodeIncrCount(ctx, last.ID)
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("verify code not match"))
		return
	}

	// 更新支付密码
	if err := o.db.UpdatePaymentPassword(ctx, userID, req.NewPaymentPassword); err != nil {
		apiresp.GinError(c, err)
		return
	}

	// 删除已使用的验证码
	_ = o.db.DelVerifyCode(ctx, last.ID)

	apiresp.GinSuccess(c, nil)
}
