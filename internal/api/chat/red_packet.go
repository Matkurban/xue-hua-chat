package chat

import (
	"encoding/json"
	"math"
	"time"

	"chat/pkg/common/db/cache"
	"chat/pkg/common/db/database"
	chatdb "chat/pkg/common/db/table/chat"
	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"chat/pkg/common/apiresp"
	"chat/pkg/common/errs"
	"chat/pkg/common/log"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// 一等公民业务消息 ContentType 常量（与 xue-hua-server pkg/apistruct 保持一致）
const (
	contentTypeRedPacketGrabNotify = 126
)

// RedPacketApi 红包功能 HTTP 处理器
type RedPacketApi struct {
	db       database.RedPacketDatabaseInterface
	cache    *cache.RedPacketCache
	im       imapi.CallerInterface
	imUserID string // 系统 IM 默认用户 ID，用于发送通知
}

// NewRedPacketApi 创建 RedPacketApi 实例
func NewRedPacketApi(db database.RedPacketDatabaseInterface, c *cache.RedPacketCache, im imapi.CallerInterface, imUserID string) *RedPacketApi {
	return &RedPacketApi{db: db, cache: c, im: im, imUserID: imUserID}
}

// ─── 请求 / 响应结构 ─────────────────────────────────────────────────────────

type SendRedPacketReq struct {
	Greeting     string  `json:"greeting"`
	TargetUserID string  `json:"targetUserID"` // 单人红包必填
	ConvID       string  `json:"convID" binding:"required"`
	TotalAmount  float64 `json:"totalAmount" binding:"required,gt=0"`
	PacketType   int32   `json:"packetType" binding:"oneof=0 1 2"` // 0=单人 1=拼手气 2=均等
	TotalCount   int32   `json:"totalCount" binding:"required,min=1,max=100"`
}

type SendRedPacketResp struct {
	PacketID string `json:"packetID"`
}

type GrabRedPacketReq struct {
	PacketID string `json:"packetID" binding:"required"`
}

type GrabRedPacketResp struct {
	Amount float64 `json:"amount"` // 实际领取积分
}

type GetRedPacketDetailReq struct {
	PacketID string `json:"packetID" binding:"required"`
}

type RedPacketGrabInfo struct {
	GrabberID  string    `json:"grabberID"`
	Nickname   string    `json:"nickname"`
	FaceURL    string    `json:"faceURL"`
	Amount     float64   `json:"amount"`
	CreateTime time.Time `json:"createTime"`
}

type GetRedPacketDetailResp struct {
	ExpireAt       time.Time           `json:"expireAt"`
	PacketID       string              `json:"packetID"`
	SenderID       string              `json:"senderID"`
	SenderNickname string              `json:"senderNickname"`
	SenderFaceURL  string              `json:"senderFaceURL"`
	Greeting       string              `json:"greeting"`
	Grabs          []RedPacketGrabInfo `json:"grabs"`
	TotalAmount    float64             `json:"totalAmount"`
	GrabbedAmount  float64             `json:"grabbedAmount"`
	MyGrabAmount   float64             `json:"myGrabAmount"` // 0 = 未领取
	PacketType     int32               `json:"packetType"`
	TotalCount     int32               `json:"totalCount"`
	GrabbedCount   int32               `json:"grabbedCount"`
	Status         int32               `json:"status"`
}

type GetPointsBalanceResp struct {
	Balance float64 `json:"balance"`
}

type GetPointsTransactionsReq struct {
	PageNumber int32  `json:"pageNumber"`
	ShowNumber int32  `json:"showNumber"`
	TxType     int32  `json:"txType"`    // 交易类型筛选：1=发红包 2=收红包 3=退款 4=管理加 5=管理减，0=不筛选
	StartTime  int64  `json:"startTime"` // 起始时间（毫秒时间戳），0=不筛选
	EndTime    int64  `json:"endTime"`   // 结束时间（毫秒时间戳），0=不筛选
	Keyword    string `json:"keyword"`   // 关键字模糊搜索（匹配 relatedID/remark）
}

type GetPointsTransactionsResp struct {
	Total        int64                       `json:"total"`
	Transactions []*chatdb.PointsTransaction `json:"transactions"`
}

// ─── 处理器 ─────────────────────────────────────────────────────────────────

// SendRedPacket POST /red_packet/send
func (o *RedPacketApi) SendRedPacket(c *gin.Context) {
	var req SendRedPacketReq
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

	if req.PacketType == chatdb.RedPacketTypeSingle {
		if req.TargetUserID == "" {
			apiresp.GinError(c, errs.ErrArgs.WrapMsg("targetUserID required for single packet"))
			return
		}
		req.TotalCount = 1
	}

	// 确保金额精度不超过两位小数，且每人至少可分到 0.01
	if math.IsNaN(req.TotalAmount) || math.IsInf(req.TotalAmount, 0) {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid totalAmount"))
		return
	}
	req.TotalAmount = math.Round(req.TotalAmount*100) / 100
	if req.TotalAmount < float64(req.TotalCount)*0.01 {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("totalAmount must be at least 0.01 per packet"))
		return
	}

	packetID := uuid.New().String()
	now := time.Now()
	packet := &chatdb.RedPacket{
		PacketID:     packetID,
		SenderID:     userID,
		ConvID:       req.ConvID,
		PacketType:   req.PacketType,
		TotalAmount:  req.TotalAmount,
		TotalCount:   req.TotalCount,
		Greeting:     req.Greeting,
		TargetUserID: req.TargetUserID,
		Status:       chatdb.RedPacketStatusActive,
		ExpireAt:     now.Add(24 * time.Hour),
		CreateTime:   now,
		UpdateTime:   now,
	}
	senderTx := &chatdb.PointsTransaction{
		TxID:       uuid.New().String(),
		UserID:     userID,
		Amount:     -req.TotalAmount,
		TxType:     chatdb.PointsTxTypeSend,
		RelatedID:  packetID,
		Remark:     "发送红包",
		CreateTime: now,
	}

	if err = o.db.SendRedPacketTx(ctx, packet, senderTx); err != nil {
		apiresp.GinError(c, err)
		return
	}

	// 初始化 Redis 预分配金额列表
	luckDraw := req.PacketType == chatdb.RedPacketTypeLuck
	if o.cache != nil {
		if err = o.cache.InitPacket(ctx, packetID, req.TotalAmount, req.TotalCount, luckDraw); err != nil {
			// Redis 失败不回滚 DB，记录错误日志即可（降级时走 DB 逻辑）
			log.ZWarn(ctx, "SendRedPacket InitPacket cache failed", err)
		}
	}

	// 推送 points_adjusted 通知给发送者（多端同步）
	// 注意：必须用 c（Gin Context）而非 c.Request.Context()，
	// 否则 GinParseOperationID 存入 c.Keys 的 operationID 会丢失，
	// 导致 IM server 拒绝请求。
	if imToken, err2 := o.im.GetAdminTokenCache(c, o.imUserID); err2 == nil {
		newBalance, _ := o.db.GetPointsBalance(ctx, userID)
		pointsData, _ := json.Marshal(map[string]any{
			"userID":  userID,
			"balance": newBalance,
		})
		imCtx := mctx.WithApiToken(c, imToken)
		_ = o.im.SendBusinessNotification(imCtx,
			imapi.NotifyKeyPointsAdjusted,
			string(pointsData),
			o.imUserID,
			userID,
		)
	}

	apiresp.GinSuccess(c, &SendRedPacketResp{PacketID: packetID})
}

// GrabRedPacket POST /red_packet/grab
func (o *RedPacketApi) GrabRedPacket(c *gin.Context) {
	var req GrabRedPacketReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	grabberID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ctx := c.Request.Context()

	// 查询红包是否存在且可领
	packet, err := o.db.GetRedPacket(ctx, req.PacketID)
	if err != nil {
		apiresp.GinError(c, errs.ErrRecordNotFound.WrapMsg("red packet not found"))
		return
	}
	if packet.Status != chatdb.RedPacketStatusActive {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("red packet not active"))
		return
	}
	if time.Now().After(packet.ExpireAt) {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("red packet expired"))
		return
	}
	// 单人红包校验
	if packet.PacketType == chatdb.RedPacketTypeSingle && packet.TargetUserID != grabberID {
		apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("not the target recipient"))
		return
	}

	// 通过 Redis Lua 原子抢
	if o.cache == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("cache not available"))
		return
	}
	amount, err := o.cache.GrabPacket(ctx, req.PacketID, grabberID)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("grab failed, please retry"))
		return
	}
	if amount == 0 {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("already grabbed"))
		return
	}
	if amount == -1 {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("red packet exhausted"))
		return
	}

	// 持久化领取记录 + 积分事务
	now := time.Now()
	grab := &chatdb.RedPacketGrab{
		GrabID:     uuid.New().String(),
		PacketID:   req.PacketID,
		GrabberID:  grabberID,
		Amount:     amount,
		CreateTime: now,
	}
	grabberTx := &chatdb.PointsTransaction{
		TxID:       uuid.New().String(),
		UserID:     grabberID,
		Amount:     amount,
		TxType:     chatdb.PointsTxTypeReceive,
		RelatedID:  req.PacketID,
		Remark:     "领取红包",
		CreateTime: now,
	}

	if err = o.db.GrabRedPacketTx(ctx, packet, grab, nil, grabberTx); err != nil {
		if rbErr := o.cache.RollbackGrab(ctx, req.PacketID, grabberID, amount); rbErr != nil {
			log.ZError(ctx, "GrabRedPacket RollbackGrab failed", rbErr, "packetID", req.PacketID, "grabberID", grabberID, "amount", amount)
		}
		log.ZError(ctx, "GrabRedPacketTx failed", err, "packetID", req.PacketID, "grabberID", grabberID)
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("record failed, please retry"))
		return
	}

	// 检查是否已全部领完（领完后更新状态）
	updatedPacket, _ := o.db.GetRedPacket(ctx, req.PacketID)
	if updatedPacket != nil && updatedPacket.GrabbedCount >= updatedPacket.TotalCount {
		_ = o.db.MarkRedPacketExhausted(ctx, req.PacketID)
	}

	// 获取领取人昵称和头像
	// 注意：必须用 c（Gin Context）以保留 operationID，否则 IM server 拒绝请求。
	var grabberName, grabberFaceURL string
	var senderName string
	imToken, tokenErr := o.im.GetAdminTokenCache(c, o.imUserID)
	if tokenErr != nil {
		log.ZWarn(ctx, "GrabRedPacket GetAdminTokenCache failed", tokenErr)
	} else {
		imCtx := mctx.WithApiToken(c, imToken)
		// 批量获取领取人和发送人信息
		if users, err2 := o.im.GetUsersInfo(imCtx, []string{grabberID, packet.SenderID}); err2 == nil {
			for _, u := range users {
				if u == nil {
					continue
				}
				if u.UserID == grabberID {
					grabberName = u.Nickname
					grabberFaceURL = u.FaceURL
				}
				if u.UserID == packet.SenderID {
					senderName = u.Nickname
				}
			}
		}
	}
	if grabberName == "" {
		grabberName = grabberID
	}
	if senderName == "" {
		senderName = packet.SenderID
	}

	// 发送红包领取提示消息到会话（私聊或群聊），作为真正的 IM 消息出现在聊天记录中。
	// convID 格式：si_userA_userB（单聊）或 sg_groupID（群聊）
	if tokenErr == nil {
		notifyCtx := mctx.WithApiToken(c, imToken)
		msgReq := &imapi.SendCustomMsgReq{
			ContentType:      contentTypeRedPacketGrabNotify,
			SenderPlatformID: 5, // constant.LinuxPlatformID (服务端)
			// 按 apistruct.RedPacketGrabNotifyElem 字段扁平填充
			Content: map[string]any{
				"packetID":       req.PacketID,
				"grabberID":      grabberID,
				"grabberName":    grabberName,
				"grabberFaceURL": grabberFaceURL,
				"senderID":       packet.SenderID,
				"senderName":     senderName,
				"amount":         amount,
			},
		}
		// 根据 convID 前缀判断会话类型
		convID := packet.ConvID
		if len(convID) > 3 && convID[:3] == "sg_" {
			// 群聊：sg_<groupID>
			// sendID 使用系统账号，消息发到群里所有人可见
			msgReq.SendID = o.imUserID
			msgReq.GroupID = convID[3:]
			msgReq.SessionType = 3 // constant.ReadGroupChatType
			msgReq.SenderNickname = "系统通知"
		} else if len(convID) > 3 && convID[:3] == "si_" {
			// 单聊：si_<userA>_<userB>
			// sendID 设为领取人，recvID 设为红包发送者
			// 这样消息会出现在领取人与发送者的私聊会话中
			msgReq.SendID = grabberID
			msgReq.SenderNickname = grabberName
			msgReq.SenderFaceURL = grabberFaceURL
			msgReq.RecvID = packet.SenderID
			msgReq.SessionType = 1 // constant.SingleChatType
		}
		if err2 := o.im.SendCustomMessage(notifyCtx, msgReq); err2 != nil {
			log.ZWarn(ctx, "GrabRedPacket SendCustomMessage failed", err2)
		}
	}

	// 同时保留 business notification 用于刷新红包气泡状态（已领取/已领完等）
	notifyData, _ := json.Marshal(map[string]interface{}{
		"packetID":       req.PacketID,
		"grabberID":      grabberID,
		"grabberName":    grabberName,
		"grabberFaceURL": grabberFaceURL,
		"amount":         amount,
		"convID":         packet.ConvID,
	})
	if tokenErr == nil {
		bnCtx := mctx.WithApiToken(c, imToken)
		_ = o.im.SendBusinessNotification(bnCtx,
			imapi.NotifyKeyRedPacketGrabbed,
			string(notifyData),
			packet.SenderID,
			grabberID,
		)
	}

	// 查询领取后余额，推送 points_adjusted 通知给领取人（多端同步）
	if tokenErr == nil {
		newBalance, _ := o.db.GetPointsBalance(ctx, grabberID)
		pointsData, _ := json.Marshal(map[string]any{
			"userID":  grabberID,
			"balance": newBalance,
		})
		ptCtx := mctx.WithApiToken(c, imToken)
		_ = o.im.SendBusinessNotification(ptCtx,
			imapi.NotifyKeyPointsAdjusted,
			string(pointsData),
			o.imUserID,
			grabberID,
		)
	}

	apiresp.GinSuccess(c, &GrabRedPacketResp{Amount: amount})
}

// GetRedPacketDetail POST /red_packet/detail
func (o *RedPacketApi) GetRedPacketDetail(c *gin.Context) {
	var req GetRedPacketDetailReq
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

	packet, err := o.db.GetRedPacket(ctx, req.PacketID)
	if err != nil {
		apiresp.GinError(c, errs.ErrRecordNotFound.WrapMsg("not found"))
		return
	}
	grabs, err := o.db.ListGrabsByPacket(ctx, req.PacketID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	// 批量获取用户信息（发送人 + 所有领取人）
	allUserIDs := make([]string, 0, len(grabs)+1)
	allUserIDs = append(allUserIDs, packet.SenderID)
	for _, g := range grabs {
		allUserIDs = append(allUserIDs, g.GrabberID)
	}
	type userBasic struct{ Nickname, FaceURL string }
	userInfoMap := make(map[string]userBasic)
	// 注意：必须传 c（Gin Context）以保留 operationID，否则 IM server 会拒绝请求
	imToken, err := o.im.GetAdminTokenCache(c, o.imUserID)
	if err != nil {
		log.ZWarn(ctx, "GetRedPacketDetail GetAdminTokenCache failed", err)
	} else {
		imCtx := mctx.WithApiToken(c, imToken)
		if users, err2 := o.im.GetUsersInfo(imCtx, allUserIDs); err2 == nil {
			for _, u := range users {
				if u != nil {
					userInfoMap[u.UserID] = userBasic{u.Nickname, u.FaceURL}
				}
			}
		} else {
			log.ZWarn(ctx, "GetRedPacketDetail GetUsersInfo failed", err2)
		}
	}
	senderInfo := userInfoMap[packet.SenderID]

	grabInfos := make([]RedPacketGrabInfo, 0, len(grabs))
	var myAmount float64
	for _, g := range grabs {
		info := RedPacketGrabInfo{
			GrabberID:  g.GrabberID,
			Amount:     g.Amount,
			CreateTime: g.CreateTime,
		}
		if ui, ok := userInfoMap[g.GrabberID]; ok {
			info.Nickname = ui.Nickname
			info.FaceURL = ui.FaceURL
		}
		grabInfos = append(grabInfos, info)
		if g.GrabberID == userID {
			myAmount = g.Amount
		}
	}

	apiresp.GinSuccess(c, &GetRedPacketDetailResp{
		PacketID:       packet.PacketID,
		SenderID:       packet.SenderID,
		SenderNickname: senderInfo.Nickname,
		SenderFaceURL:  senderInfo.FaceURL,
		PacketType:     packet.PacketType,
		TotalAmount:    packet.TotalAmount,
		TotalCount:     packet.TotalCount,
		GrabbedAmount:  packet.GrabbedAmount,
		GrabbedCount:   packet.GrabbedCount,
		Status:         packet.Status,
		Greeting:       packet.Greeting,
		ExpireAt:       packet.ExpireAt,
		Grabs:          grabInfos,
		MyGrabAmount:   myAmount,
	})
}

// GetPointsBalance GET /points/balance
func (o *RedPacketApi) GetPointsBalance(c *gin.Context) {
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	balance, err := o.db.GetPointsBalance(c.Request.Context(), userID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &GetPointsBalanceResp{Balance: balance})
}

// GetPointsTransactions POST /points/transactions
func (o *RedPacketApi) GetPointsTransactions(c *gin.Context) {
	var req GetPointsTransactionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	userID, err := mctx.CheckUser(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if req.PageNumber <= 0 {
		req.PageNumber = 1
	}
	if req.ShowNumber <= 0 {
		req.ShowNumber = 20
	}
	p := newPage(req.PageNumber, req.ShowNumber)
	var startTime, endTime time.Time
	if req.StartTime > 0 {
		startTime = time.UnixMilli(req.StartTime)
	}
	if req.EndTime > 0 {
		endTime = time.UnixMilli(req.EndTime)
	}
	total, txs, err := o.db.SearchUserTransactionsFiltered(c.Request.Context(), userID, req.TxType, startTime, endTime, req.Keyword, p)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &GetPointsTransactionsResp{Total: total, Transactions: txs})
}

// ─── Admin 接口 ──────────────────────────────────────────────────────────────

type AdminAdjustPointsReq struct {
	UserID string  `json:"userID" binding:"required"`
	Amount float64 `json:"amount" binding:"required"` // 正数加，负数减
	Remark string  `json:"remark"`
}

type AdminListPointsReq struct {
	Keyword    string `json:"keyword"`
	PageNumber int32  `json:"pageNumber"`
	ShowNumber int32  `json:"showNumber"`
}

type AdminListRedPacketsReq struct {
	Keyword    string `json:"keyword"`
	PageNumber int32  `json:"pageNumber"`
	ShowNumber int32  `json:"showNumber"`
}

type AdminListGrabsReq struct {
	Keyword    string `json:"keyword"`
	PageNumber int32  `json:"pageNumber"`
	ShowNumber int32  `json:"showNumber"`
}

// AdminAdjustPoints POST /admin/points/adjust
func (o *RedPacketApi) AdminAdjustPoints(c *gin.Context) {
	var req AdminAdjustPointsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	operatorID, err := mctx.CheckAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	now := time.Now()
	// 确保金额精度不超过两位小数
	req.Amount = math.Round(req.Amount*100) / 100
	txType := int32(chatdb.PointsTxTypeAdminAdd)
	remark := req.Remark
	if req.Amount < 0 {
		txType = chatdb.PointsTxTypeAdminSub
	}
	if remark == "" {
		remark = "管理员调整"
	}
	adminTx := &chatdb.PointsTransaction{
		TxID:       uuid.New().String(),
		UserID:     req.UserID,
		Amount:     req.Amount,
		TxType:     txType,
		RelatedID:  operatorID,
		Remark:     remark,
		CreateTime: now,
	}
	if err = o.db.AdminAdjustPoints(c.Request.Context(), req.UserID, req.Amount, adminTx); err != nil {
		apiresp.GinError(c, err)
		return
	}
	// 查询调整后余额并推送给客户端
	newBalance, _ := o.db.GetPointsBalance(c.Request.Context(), req.UserID)
	notifyData, _ := json.Marshal(map[string]any{
		"userID":  req.UserID,
		"balance": newBalance,
		"amount":  req.Amount,
		"remark":  remark,
	})
	// 注意：必须用 c（Gin Context）以保留 operationID，否则 IM server 拒绝请求。
	if imToken, err2 := o.im.ImAdminTokenWithDefaultAdmin(c); err2 == nil {
		notifyCtx := mctx.WithApiToken(c, imToken)
		_ = o.im.SendBusinessNotification(notifyCtx,
			imapi.NotifyKeyPointsAdjusted,
			string(notifyData),
			o.imUserID,
			req.UserID,
		)
	}
	apiresp.GinSuccess(c, nil)
}

// AdminListPoints POST /admin/points/list
func (o *RedPacketApi) AdminListPoints(c *gin.Context) {
	var req AdminListPointsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if req.PageNumber <= 0 {
		req.PageNumber = 1
	}
	if req.ShowNumber <= 0 {
		req.ShowNumber = 20
	}
	p := newPage(req.PageNumber, req.ShowNumber)
	total, users, err := o.db.SearchUserPoints(c.Request.Context(), req.Keyword, p)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, map[string]any{"total": total, "list": users})
}

// AdminListRedPackets POST /admin/red_packets/list
func (o *RedPacketApi) AdminListRedPackets(c *gin.Context) {
	var req AdminListRedPacketsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if req.PageNumber <= 0 {
		req.PageNumber = 1
	}
	if req.ShowNumber <= 0 {
		req.ShowNumber = 20
	}
	p := newPage(req.PageNumber, req.ShowNumber)
	total, packets, err := o.db.SearchRedPackets(c.Request.Context(), req.Keyword, p)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, map[string]any{"total": total, "packets": packets})
}

// AdminListGrabs POST /admin/red_packets/grabs
func (o *RedPacketApi) AdminListGrabs(c *gin.Context) {
	var req AdminListGrabsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if req.PageNumber <= 0 {
		req.PageNumber = 1
	}
	if req.ShowNumber <= 0 {
		req.ShowNumber = 20
	}
	p := newPage(req.PageNumber, req.ShowNumber)
	total, grabs, err := o.db.SearchGrabs(c.Request.Context(), req.Keyword, p)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, map[string]any{"total": total, "grabs": grabs})
}

// AdminListTransactions POST /admin/points/transactions
func (o *RedPacketApi) AdminListTransactions(c *gin.Context) {
	var req struct {
		UserID     string `json:"userID"`
		TxType     int32  `json:"txType"`
		PageNumber int32  `json:"pageNumber"`
		ShowNumber int32  `json:"showNumber"`
		Pagination *struct {
			PageNumber int32 `json:"pageNumber"`
			ShowNumber int32 `json:"showNumber"`
		} `json:"pagination"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	// 兼容嵌套 pagination 和平铺两种格式
	pageNum, showNum := req.PageNumber, req.ShowNumber
	if req.Pagination != nil {
		pageNum = req.Pagination.PageNumber
		showNum = req.Pagination.ShowNumber
	}
	if pageNum <= 0 {
		pageNum = 1
	}
	if showNum <= 0 {
		showNum = 20
	}
	p := newPage(pageNum, showNum)
	total, txs, err := o.db.SearchAllTransactions(c.Request.Context(), req.UserID, req.TxType, p)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, map[string]any{"total": total, "list": txs})
}
