package chat

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"chat/pkg/common/apistruct"
	"chat/pkg/common/constant"
	chatdb "chat/pkg/common/db/table/chat"
	"chat/pkg/common/mctx"
	"chat/pkg/common/qrlogin"
	"chat/pkg/protocol/admin"
	"chat/pkg/common/apiresp"
	"chat/pkg/common/errs"
	"chat/pkg/common/log"
	"github.com/gin-gonic/gin"
	constantpb "github.com/openimsdk/protocol/constant"
)

type qrCreateLimiter struct {
	mu          sync.Mutex
	entries     map[string][]time.Time
	lastCleanup time.Time
}

func newQrCreateLimiter() *qrCreateLimiter {
	return &qrCreateLimiter{entries: make(map[string][]time.Time)}
}

func (l *qrCreateLimiter) allow(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-window)
	// 定期清理过期 key，防止按 sessionID / IP 限流导致 map 无限增长
	if len(l.entries) > 1024 && now.Sub(l.lastCleanup) > window {
		for k, ts := range l.entries {
			keep := ts[:0]
			for _, t := range ts {
				if t.After(cutoff) {
					keep = append(keep, t)
				}
			}
			if len(keep) == 0 {
				delete(l.entries, k)
			} else {
				l.entries[k] = keep
			}
		}
		l.lastCleanup = now
	}
	times := l.entries[key]
	filtered := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) >= limit {
		l.entries[key] = filtered
		return false
	}
	filtered = append(filtered, now)
	l.entries[key] = filtered
	return true
}

func (o *Api) QrLoginCreate(c *gin.Context) {
	var req apistruct.QrLoginCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.QrLoginManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("qr login not configured"))
		return
	}
	if !qrlogin.IsDesktopPlatform(req.Platform) {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("platform must be desktop"))
		return
	}
	ip, err := o.GetClientIP(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.qrCreateLimiter != nil && !o.qrCreateLimiter.allow(ip, 10, time.Minute) {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("too many qr login requests"))
		return
	}
	session, sessionID, err := o.QrLoginManager.Create(c, req.Platform, req.DeviceID, ip)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &apistruct.QrLoginCreateResp{
		SessionID: sessionID,
		ExpireAt:  session.ExpireAt,
	})
}

func (o *Api) QrLoginStatus(c *gin.Context) {
	var sessionID string
	if c.Request.Method == http.MethodPost {
		var req apistruct.QrLoginSessionReq
		if err := c.ShouldBindJSON(&req); err != nil {
			apiresp.GinError(c, err)
			return
		}
		sessionID = req.SessionID
	} else {
		sessionID = c.Query("sessionID")
	}
	if sessionID == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("sessionID is required"))
		return
	}
	if o.QrLoginManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("qr login not configured"))
		return
	}
	if o.qrStatusLimiter != nil && !o.qrStatusLimiter.allow(sessionID, 2, time.Second) {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("too many qr login status requests"))
		return
	}
	session, err := o.QrLoginManager.Get(c, sessionID)
	if err != nil {
		if errors.Is(err, qrlogin.ErrSessionNotFound) {
			apiresp.GinSuccess(c, &apistruct.QrLoginStatusResp{Status: qrlogin.StatusExpired})
			return
		}
		apiresp.GinError(c, err)
		return
	}
	resp := &apistruct.QrLoginStatusResp{
		Status:   session.Status,
		Platform: session.Platform,
		ClientIP: session.ClientIP,
	}
	switch session.Status {
	case qrlogin.StatusConfirmed:
		consumed, consumeErr := o.QrLoginManager.TryConsumeConfirmed(c, sessionID)
		if consumeErr != nil {
			if errors.Is(consumeErr, qrlogin.ErrAlreadyConsumed) {
				apiresp.GinSuccess(c, &apistruct.QrLoginStatusResp{Status: qrlogin.StatusConsumed})
				return
			}
			if errors.Is(consumeErr, qrlogin.ErrNotConfirmed) {
				apiresp.GinSuccess(c, resp)
				return
			}
			apiresp.GinError(c, consumeErr)
			return
		}
		tokens, tokenErr := o.issueQrLoginTokens(c, consumed)
		if tokenErr != nil {
			if rollbackErr := o.QrLoginManager.RollbackConsumed(c, sessionID); rollbackErr != nil {
				log.ZWarn(c, "qr login rollback consumed failed", rollbackErr, "sessionID", sessionID)
			}
			log.ZError(c, "qr login issue tokens failed", tokenErr, "sessionID", sessionID)
			apiresp.GinError(c, tokenErr)
			return
		}
		o.recordQrLogin(c, consumed)
		resp.UserID = tokens.UserID
		resp.ImToken = tokens.ImToken
		resp.ChatToken = tokens.ChatToken
		resp.Status = qrlogin.StatusConfirmed
	case qrlogin.StatusConsumed:
		resp.Status = qrlogin.StatusConsumed
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) QrLoginScan(c *gin.Context) {
	var req apistruct.QrLoginSessionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.QrLoginManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("qr login not configured"))
		return
	}
	session, err := o.QrLoginManager.Scan(c, req.SessionID, userID)
	if err != nil {
		if errors.Is(err, qrlogin.ErrSessionNotFound) {
			apiresp.GinError(c, errs.ErrArgs.WrapMsg("session expired"))
			return
		}
		if errors.Is(err, qrlogin.ErrInvalidStatus) || errors.Is(err, qrlogin.ErrScannedByOther) {
			apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
			return
		}
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &apistruct.QrLoginConfirmResp{
		Platform: session.Platform,
		ClientIP: session.ClientIP,
		DeviceID: session.DeviceID,
	})
}

func (o *Api) QrLoginConfirm(c *gin.Context) {
	var req apistruct.QrLoginConfirmReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.QrLoginManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("qr login not configured"))
		return
	}
	session, err := o.QrLoginManager.Confirm(c, req.SessionID, userID)
	if err != nil {
		if errors.Is(err, qrlogin.ErrSessionNotFound) {
			apiresp.GinError(c, errs.ErrArgs.WrapMsg("session expired"))
			return
		}
		if errors.Is(err, qrlogin.ErrInvalidStatus) || errors.Is(err, qrlogin.ErrScannedByOther) {
			apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
			return
		}
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &apistruct.QrLoginConfirmResp{
		Platform: session.Platform,
		ClientIP: session.ClientIP,
		DeviceID: session.DeviceID,
	})
}

func (o *Api) QrLoginCancel(c *gin.Context) {
	var req apistruct.QrLoginSessionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.QrLoginManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("qr login not configured"))
		return
	}
	if err := o.QrLoginManager.Cancel(c, req.SessionID); err != nil {
		if errors.Is(err, qrlogin.ErrSessionNotFound) || errors.Is(err, qrlogin.ErrInvalidStatus) {
			apiresp.GinSuccess(c, gin.H{})
			return
		}
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, gin.H{})
}

type qrLoginTokens struct {
	UserID    string
	ImToken   string
	ChatToken string
}

func (o *Api) issueQrLoginTokens(c *gin.Context, session *qrlogin.Session) (*qrLoginTokens, error) {
	if _, err := o.adminClient.CheckLoginForbidden(c, &admin.CheckLoginForbiddenReq{
		Ip:     session.ClientIP,
		UserID: session.UserID,
	}); err != nil {
		return nil, err
	}
	chatTokenResp, err := o.adminClient.CreateToken(c, &admin.CreateTokenReq{
		UserID:   session.UserID,
		UserType: constant.NormalUser,
	})
	if err != nil {
		return nil, err
	}
	adminToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		return nil, err
	}
	apiCtx := mctx.WithApiToken(c, adminToken)
	imToken, err := o.imApiCaller.GetUserToken(apiCtx, session.UserID, session.Platform)
	if err != nil {
		return nil, err
	}
	return &qrLoginTokens{
		UserID:    session.UserID,
		ImToken:   imToken,
		ChatToken: chatTokenResp.Token,
	}, nil
}

func (o *Api) recordQrLogin(c *gin.Context, session *qrlogin.Session) {
	if o.chatDatabase == nil || session == nil || session.UserID == "" {
		return
	}
	record := &chatdb.UserLoginRecord{
		UserID:    session.UserID,
		LoginTime: time.Now(),
		IP:        session.ClientIP,
		DeviceID:  session.DeviceID,
		Platform:  constantpb.PlatformIDToName(int(session.Platform)),
	}
	if err := o.chatDatabase.LoginRecord(c, record, nil); err != nil {
		log.ZWarn(c, "qr login record failed", err, "userID", session.UserID)
	}
}
