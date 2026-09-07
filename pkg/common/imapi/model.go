package imapi

import "github.com/openimsdk/protocol/sdkws"

// SendSingleMsgReq defines the structure for sending a message to multiple recipients.
type SendSingleMsgReq struct {
	// groupMsg should appoint sendID
	SendID          string                 `json:"sendID"`
	Content         string                 `json:"content" binding:"required"`
	OfflinePushInfo *sdkws.OfflinePushInfo `json:"offlinePushInfo"`
	Ex              string                 `json:"ex"`
}
type SendSingleMsgResp struct{}

// SendBusinessNotificationReq is the request for /msg/send_business_notification.
type SendBusinessNotificationReq struct {
	Key              string `json:"key"`
	Data             string `json:"data"`
	SendUserID       string `json:"sendUserID" binding:"required"`
	RecvUserID       string `json:"recvUserID"`
	SendMsg          bool   `json:"sendMsg"`
	ReliabilityLevel int    `json:"reliabilityLevel"`
}

type SendBusinessNotificationResp struct{}

// SendCustomMsgReq 用于通过 /msg/send_msg 发送自定义消息到私聊或群聊。
// 结构对齐 xue-hua-server 的 apistruct.SendMsgReq。
type SendCustomMsgReq struct {
	RecvID           string                 `json:"recvID"`
	SendID           string                 `json:"sendID" binding:"required"`
	GroupID          string                 `json:"groupID"`
	SessionType      int32                  `json:"sessionType" binding:"required"`
	ContentType      int32                  `json:"contentType" binding:"required"`
	Content          map[string]any         `json:"content" binding:"required"`
	SenderNickname   string                 `json:"senderNickname"`
	SenderFaceURL    string                 `json:"senderFaceURL"`
	SenderPlatformID int32                  `json:"senderPlatformID"`
	OfflinePushInfo  *sdkws.OfflinePushInfo `json:"offlinePushInfo"`
}
type SendCustomMsgResp struct {
	ServerMsgID string `json:"serverMsgID"`
	ClientMsgID string `json:"clientMsgID"`
	SendTime    int64  `json:"sendTime"`
}

// ModifyMsgContentReq merges fields into stored message Content JSON (admin).
type ModifyMsgContentReq struct {
	ConversationID string         `json:"conversationID"`
	ClientMsgID    string         `json:"clientMsgID"`
	MergeContent   map[string]any `json:"mergeContent"`
}

type ModifyMsgContentResp struct {
	Seq int64 `json:"seq"`
}

type BatchSendMsgReq struct {
	Content          map[string]any         `json:"content"`
	OfflinePushInfo  *sdkws.OfflinePushInfo `json:"offlinePushInfo"`
	SendID           string                 `json:"sendID"`
	SenderNickname   string                 `json:"senderNickname"`
	SenderFaceURL    string                 `json:"senderFaceURL"`
	Ex               string                 `json:"ex"`
	RecvIDs          []string               `json:"recvIDs"`
	SenderPlatformID int32                  `json:"senderPlatformID"`
	ContentType      int32                  `json:"contentType"`
	SessionType      int32                  `json:"sessionType"`
	IsSendAll        bool                   `json:"isSendAll"`
	IsOnlineOnly     bool                   `json:"isOnlineOnly"`
	NotOfflinePush   bool                   `json:"notOfflinePush"`
}

type BatchSendMsgResult struct {
	ServerMsgID string `json:"serverMsgID"`
	ClientMsgID string `json:"clientMsgID"`
	SendTime    int64  `json:"sendTime"`
	RecvID      string `json:"recvID"`
}

type BatchSendMsgResp struct {
	Results       []*BatchSendMsgResult `json:"results"`
	FailedUserIDs []string              `json:"failedUserIDs"`
}

type BatchRevokeMsgItem struct {
	RecvID      string `json:"recvID"`
	ClientMsgID string `json:"clientMsgID"`
}

type BatchRevokeMsgReq struct {
	SendID      string               `json:"sendID"`
	SessionType int32                `json:"sessionType"`
	Items       []BatchRevokeMsgItem `json:"items"`
}

type BatchRevokeMsgResp struct {
	SuccessCount  int      `json:"successCount"`
	FailedCount   int      `json:"failedCount"`
	FailedUserIDs []string `json:"failedUserIDs"`
}

type GetGroupMemberUserIDsReq struct {
	GroupID string `json:"groupID"`
}

type GetGroupMemberUserIDsResp struct {
	UserIDs []string `json:"userIDs"`
}

// Notification key constants for moments & favorites.
const (
	// 红包相关通知 key
	NotifyKeyRedPacketGrabbed = "red_packet_grabbed" // XXX 领取了你的红包
	NotifyKeyPointsAdjusted   = "points_adjusted"    // 管理员调整积分

	NotifyKeyMomentCreated        = "moment_created"
	NotifyKeyMomentDeleted        = "moment_deleted"
	NotifyKeyMomentLiked          = "moment_liked"
	NotifyKeyMomentUnliked        = "moment_unliked"
	NotifyKeyMomentCommented      = "moment_commented"
	NotifyKeyMomentCommentDeleted = "moment_comment_deleted"
	NotifyKeyFavoriteAdded        = "favorite_added"
	NotifyKeyFavoriteRemoved      = "favorite_removed"
	NotifyKeyFavoriteUpdated      = "favorite_updated"
)
