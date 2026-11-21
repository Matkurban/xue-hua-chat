package rtc

import (
	"context"
	"encoding/json"

	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"chat/pkg/common/log"
)

const (
	ContentTypeMeetingScheduleNotify = 129
	scheduleMeetingNotifyBizID       = "schedule_meeting_notify"
)

// ScheduleNotifyPayload 预约变更/取消通知内容
type ScheduleNotifyPayload struct {
	Action         string `json:"action"` // updated|cancelled|invited|removed
	MeetingID      string `json:"meetingID"`
	OccurrenceID   string `json:"occurrenceID"`
	SeriesID       string `json:"seriesID"`
	Title          string `json:"title"`
	StartTime      int64  `json:"startTime"`
	OldStartTime   int64  `json:"oldStartTime,omitempty"`
	EndTime        int64  `json:"endTime,omitempty"`
	Timezone       string `json:"timezone"`
	Scope          string `json:"scope"`
	OperatorUserID string `json:"operatorUserID"`
	HasPassword    bool   `json:"hasPassword"`
}

// ScheduleNotifier 发送 129 + 业务推送
type ScheduleNotifier struct {
	im          imapi.CallerInterface
	adminUserID string
}

func NewScheduleNotifier(im imapi.CallerInterface, adminUserID string) *ScheduleNotifier {
	return &ScheduleNotifier{im: im, adminUserID: adminUserID}
}

// NotifyInvitees 向受邀人发送通知（排除 operator）。
func (n *ScheduleNotifier) NotifyInvitees(
	ctx context.Context, operatorUserID string, inviteeUserIDs []string,
	sourceGroupIDs, sourceConversationIDs []string, p ScheduleNotifyPayload,
) {
	if n == nil || n.im == nil {
		return
	}
	tok, err := n.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		log.ZWarn(ctx, "schedule notify token failed", err)
		return
	}
	apiCtx := mctx.WithApiToken(ctx, tok)
	if p.OperatorUserID == "" {
		p.OperatorUserID = operatorUserID
	}

	seen := map[string]struct{}{}
	for _, uid := range inviteeUserIDs {
		if uid == "" || uid == operatorUserID {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		n.sendSingle(apiCtx, uid, p)
	}

	// 群会话额外一条（可见），不重复单聊已覆盖的逻辑：群消息独立展示
	for _, gid := range sourceGroupIDs {
		if gid == "" {
			continue
		}
		n.sendGroup(apiCtx, gid, p)
	}
	for _, cid := range sourceConversationIDs {
		if stringsHasPrefix(cid, "sg_") {
			gid := cid[3:]
			if gid == "" {
				continue
			}
			if containsStr(sourceGroupIDs, gid) {
				continue
			}
			n.sendGroup(apiCtx, gid, p)
		}
	}
}

func (n *ScheduleNotifier) senderUserID(p ScheduleNotifyPayload) string {
	if p.OperatorUserID != "" {
		return p.OperatorUserID
	}
	return n.adminUserID
}

func (n *ScheduleNotifier) sendSingle(ctx context.Context, recvID string, p ScheduleNotifyPayload) {
	sendID := n.senderUserID(p)
	content := notifyContentMap(p)
	req := &imapi.SendCustomMsgReq{
		SendID:           sendID,
		RecvID:           recvID,
		SessionType:      1,
		ContentType:      ContentTypeMeetingScheduleNotify,
		SenderPlatformID: 5,
		Content:          content,
	}
	if err := n.im.SendCustomMessage(ctx, req); err != nil {
		log.ZWarn(ctx, "schedule notify msg failed", err, "recvID", recvID)
	}
	data, _ := json.Marshal(p)
	if err := n.im.SendBusinessNotification(ctx, scheduleMeetingNotifyBizID, string(data), sendID, recvID); err != nil {
		log.ZWarn(ctx, "schedule notify business failed", err, "recvID", recvID)
	}
}

func (n *ScheduleNotifier) sendGroup(ctx context.Context, groupID string, p ScheduleNotifyPayload) {
	sendID := n.senderUserID(p)
	content := notifyContentMap(p)
	req := &imapi.SendCustomMsgReq{
		SendID:           sendID,
		GroupID:          groupID,
		SessionType:      3,
		ContentType:      ContentTypeMeetingScheduleNotify,
		SenderPlatformID: 5,
		Content:          content,
	}
	if err := n.im.SendCustomMessage(ctx, req); err != nil {
		log.ZWarn(ctx, "schedule notify group msg failed", err, "groupID", groupID)
	}
}

func notifyContentMap(p ScheduleNotifyPayload) map[string]any {
	return map[string]any{
		"action":         p.Action,
		"meetingID":      p.MeetingID,
		"occurrenceID":   p.OccurrenceID,
		"seriesID":       p.SeriesID,
		"title":          p.Title,
		"startTime":      p.StartTime,
		"oldStartTime":   p.OldStartTime,
		"endTime":        p.EndTime,
		"timezone":       p.Timezone,
		"scope":          p.Scope,
		"operatorUserID": p.OperatorUserID,
		"hasPassword":    p.HasPassword,
	}
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
