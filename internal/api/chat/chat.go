package chat

import (
	"io"
	"time"

	"chat/internal/api/util"
	"chat/pkg/common/rtc"

	"chat/pkg/common/apistruct"
	"chat/pkg/common/db/database"
	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"chat/pkg/common/qrlogin"
	"chat/pkg/protocol/admin"
	chatpb "chat/pkg/protocol/chat"
	"chat/pkg/common/a2r"
	"chat/pkg/common/apiresp"
	"chat/pkg/common/errs"
	"chat/pkg/common/log"
	"github.com/gin-gonic/gin"
	constantpb "github.com/openimsdk/protocol/constant"
	"github.com/openimsdk/protocol/sdkws"
)

func New(chatClient chatpb.ChatClient, adminClient admin.AdminClient, imApiCaller imapi.CallerInterface, api *util.Api, meetingMgr *rtc.MeetingManager, conferenceMgr *rtc.ConferenceManager, rpApi *RedPacketApi, qrLoginMgr *qrlogin.Manager) *Api {
	return &Api{
		Api:               api,
		chatClient:        chatClient,
		adminClient:       adminClient,
		imApiCaller:       imApiCaller,
		MeetingManager:    meetingMgr,
		ConferenceManager: conferenceMgr,
		RedPacket:         rpApi,
		QrLoginManager:    qrLoginMgr,
		qrCreateLimiter:   newQrCreateLimiter(),
		qrStatusLimiter:   newQrCreateLimiter(),
	}
}

type Api struct {
	*util.Api
	chatClient        chatpb.ChatClient
	adminClient       admin.AdminClient
	imApiCaller       imapi.CallerInterface
	MeetingManager    *rtc.MeetingManager
	ConferenceManager *rtc.ConferenceManager
	ScheduleService   *rtc.ScheduleService
	// 红包 & 积分
	RedPacket *RedPacketApi
	// 支付密码
	PaymentPassword *PaymentPasswordApi
	// 举报
	Report *ReportApi
	// ASR 模型清单
	AsrModel *AsrModelApi
	// 桌面扫码登录
	QrLoginManager  *qrlogin.Manager
	qrCreateLimiter *qrCreateLimiter
	qrStatusLimiter *qrCreateLimiter
	chatDatabase    database.ChatDatabaseInterface
}

// ################## ACCOUNT ##################

func (o *Api) SendVerifyCode(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.SendVerifyCodeReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ip, err := o.GetClientIP(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.Ip = ip
	resp, err := o.chatClient.SendVerifyCode(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) VerifyCode(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.VerifyCode, o.chatClient)
}

func (o *Api) RegisterUser(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.RegisterUserReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ip, err := o.GetClientIP(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.Ip = ip

	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiCtx := mctx.WithApiToken(c, imToken)
	rpcCtx := o.WithAdminUser(c)

	checkResp, err := o.chatClient.CheckUserExist(rpcCtx, &chatpb.CheckUserExistReq{User: req.User})
	if err != nil {
		log.ZDebug(rpcCtx, "Not else", errs.Unwrap(err))
		apiresp.GinError(c, err)
		return
	}
	if checkResp.IsRegistered {
		isUserNotExist, err := o.imApiCaller.AccountCheckSingle(apiCtx, checkResp.Userid)
		if err != nil {
			apiresp.GinError(c, err)
			return
		}
		// if User is  not exist in SDK server. You need delete this user and register new user again.
		if isUserNotExist {
			_, err := o.chatClient.DelUserAccount(rpcCtx, &chatpb.DelUserAccountReq{UserIDs: []string{checkResp.Userid}})
			log.ZDebug(c, "Delete Succsssss", checkResp.Userid)
			if err != nil {
				apiresp.GinError(c, err)
				return
			}
		}
	}

	respRegisterUser, err := o.chatClient.RegisterUser(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	userInfo := &sdkws.UserInfo{
		UserID:     respRegisterUser.UserID,
		Nickname:   req.User.Nickname,
		FaceURL:    req.User.FaceURL,
		CreateTime: time.Now().UnixMilli(),
	}
	err = o.imApiCaller.RegisterUser(apiCtx, []*sdkws.UserInfo{userInfo})
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	if resp, err := o.adminClient.FindDefaultFriend(rpcCtx, &admin.FindDefaultFriendReq{}); err == nil {
		_ = o.imApiCaller.ImportFriend(apiCtx, respRegisterUser.UserID, resp.UserIDs)
	}
	if resp, err := o.adminClient.FindDefaultGroup(rpcCtx, &admin.FindDefaultGroupReq{}); err == nil {
		_ = o.imApiCaller.InviteToGroup(apiCtx, respRegisterUser.UserID, resp.GroupIDs)
	}
	var resp apistruct.UserRegisterResp
	if req.AutoLogin {
		resp.ImToken, err = o.imApiCaller.GetUserToken(apiCtx, respRegisterUser.UserID, req.Platform)
		if err != nil {
			apiresp.GinError(c, err)
			return
		}
	}
	resp.ChatToken = respRegisterUser.ChatToken
	resp.UserID = respRegisterUser.UserID
	apiresp.GinSuccess(c, &resp)
}

func (o *Api) Login(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.LoginReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ip, err := o.GetClientIP(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.Ip = ip
	resp, err := o.chatClient.Login(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	adminToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiCtx := mctx.WithApiToken(c, adminToken)

	imToken, err := o.imApiCaller.GetUserToken(apiCtx, resp.UserID, req.Platform)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &apistruct.LoginResp{
		ImToken:   imToken,
		UserID:    resp.UserID,
		ChatToken: resp.ChatToken,
	})
}

func (o *Api) ResetPassword(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.ResetPassword, o.chatClient)
}

func (o *Api) ChangePassword(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.ChangePasswordReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.chatClient.ChangePassword(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	err = o.imApiCaller.ForceOffLine(mctx.WithApiToken(c, imToken), req.UserID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

// DeleteMyAccount 注销当前登录账号：
// - 通过 mw.CheckToken 中间件校验调用方身份；
// - 在请求体中再次校验当前登录密码（与修改密码保持一致的校验方式）；
// - 删除 chat 端用户记录（凭证 / 账号 / 属性），同时强制下线所有 IM 端会话。
// 注意：xue-hua-server 端的 IM 用户记录会被保留（其他用户的历史关系/消息引用仍然可用），
// 但凭证已被清除，用户无法再登录；如需重新启用，需重新注册。
func (o *Api) DeleteMyAccount(c *gin.Context) {
	req, err := a2r.ParseRequest[apistruct.DeleteMyAccountReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if req.CurrentPassword == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("current password must be set"))
		return
	}
	// 通过修改密码 RPC 复用密码校验逻辑：
	// 把 NewPassword 设为一个与 CurrentPassword 不同的临时占位值，
	// ChangePassword 在 CurrentPassword 错误时会返回错误，正确才会继续。
	// 紧接着调用 DelUserAccount 删除整个账号，临时密码不会被使用。
	tmpNewPassword := req.CurrentPassword + "_to_delete"
	if _, err := o.chatClient.ChangePassword(c, &chatpb.ChangePasswordReq{
		UserID:          userID,
		CurrentPassword: req.CurrentPassword,
		NewPassword:     tmpNewPassword,
	}); err != nil {
		apiresp.GinError(c, err)
		return
	}
	rpcCtx := o.WithAdminUser(c)
	if _, err := o.chatClient.DelUserAccount(rpcCtx, &chatpb.DelUserAccountReq{
		UserIDs: []string{userID},
	}); err != nil {
		apiresp.GinError(c, err)
		return
	}
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		log.ZWarn(c, "DeleteMyAccount: get im admin token failed", err, "userID", userID)
		apiresp.GinSuccess(c, &apistruct.DeleteMyAccountResp{})
		return
	}
	if err := o.imApiCaller.ForceOffLine(mctx.WithApiToken(c, imToken), userID); err != nil {
		log.ZWarn(c, "DeleteMyAccount: force offline failed", err, "userID", userID)
	}
	apiresp.GinSuccess(c, &apistruct.DeleteMyAccountResp{})
}

// ################## USER ##################

func (o *Api) UpdateUserInfo(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.UpdateUserInfoReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	respUpdate, err := o.chatClient.UpdateUserInfo(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	var imToken string
	imToken, err = o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	var (
		nickName string
		faceURL  string
	)
	if req.Nickname != nil {
		nickName = req.Nickname.Value
	} else {
		nickName = respUpdate.NickName
	}
	if req.FaceURL != nil {
		faceURL = req.FaceURL.Value
	} else {
		faceURL = respUpdate.FaceUrl
	}
	err = o.imApiCaller.UpdateUserInfo(mctx.WithApiToken(c, imToken), req.UserID, nickName, faceURL)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, apistruct.UpdateUserInfoResp{})
}

func (o *Api) FindUserPublicInfo(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.FindUserPublicInfo, o.chatClient)
}

func (o *Api) FindUserFullInfo(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.FindUserFullInfo, o.chatClient)
}

func (o *Api) SearchUserFullInfo(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.SearchUserFullInfo, o.chatClient)
}

func (o *Api) SearchUserPublicInfo(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.SearchUserPublicInfo, o.chatClient)
}

func (o *Api) GetTokenForVideoMeeting(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.GetTokenForVideoMeeting, o.chatClient)
}

// ################## APPLET ##################

func (o *Api) FindApplet(c *gin.Context) {
	a2r.Call(c, admin.AdminClient.FindApplet, o.adminClient)
}

// ################## CONFIG ##################

func (o *Api) GetClientConfig(c *gin.Context) {
	a2r.Call(c, admin.AdminClient.GetClientConfig, o.adminClient)
}

// ################## CALLBACK ##################

func (o *Api) ServerCallback(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req := &chatpb.ServerCallbackReq{
		Command: c.Query(constantpb.CallbackCommand),
		Body:    string(body),
	}
	if _, err := o.chatClient.ServerCallback(c, req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, nil)
}

func (o *Api) SearchFriend(c *gin.Context) {
	req, err := a2r.ParseRequest[struct {
		UserID string `json:"userID"`
		chatpb.SearchUserInfoReq
	}](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if req.UserID == "" {
		req.UserID = mctx.GetOpUserID(c)
	}
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	userIDs, err := o.imApiCaller.FriendUserIDs(mctx.WithApiToken(c, imToken), req.UserID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if len(userIDs) == 0 {
		apiresp.GinSuccess(c, &chatpb.SearchUserInfoResp{})
		return
	}
	req.SearchUserInfoReq.UserIDs = userIDs
	resp, err := o.chatClient.SearchUserInfo(c, &req.SearchUserInfoReq)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) LatestApplicationVersion(c *gin.Context) {
	a2r.Call(c, admin.AdminClient.LatestApplicationVersion, o.adminClient)
}
