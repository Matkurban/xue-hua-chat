package rtc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"gitee.com/matkurban/openimsdk-tools/log"
)

const rtcSignalingBusinessID = "rtc_signaling"
const customMsgContentType = 110
const singleChatSessionType = 1

// HandleAbnormalLeave 用户未通过 meeting/leave|end 就从 LiveKit 离开（或从未进房超时）
func (m *MeetingManager) HandleAbnormalLeave(ctx context.Context, roomID, userID, reason string) error {
	log.ZInfo(ctx, "meeting abnormal leave", "roomID", roomID, "userID", userID, "reason", reason)

	var roomBefore *MeetingRoom
	err := m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		active, err := m.store.IsParticipantActive(lockCtx, roomID, userID)
		if err != nil {
			return err
		}
		if !active {
			return nil
		}

		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		roomBefore = new(*room)

		if err := m.leaveMeetingLocked(lockCtx, room, userID); err != nil {
			return err
		}

		if len(roomBefore.InviteeUserIDs) == 1 {
			peerID := abnormalLeavePeerID(roomBefore, userID)
			if peerID != "" {
				if _, err := m.endMeetingLocked(lockCtx, roomBefore.RoomID, roomBefore.CreatorUserID); err != nil {
					log.ZWarn(lockCtx, "abnormal leave end meeting failed", err, "roomID", roomID)
				}
				m.sendHangupSignal(lockCtx, roomBefore, userID, peerID)
			}
		}
		return nil
	})
	return err
}

func (m *MeetingManager) leaveMeetingLocked(ctx context.Context, room *MeetingRoom, userID string) error {
	delete(room.Participants, userID)
	_ = m.store.rdb.Del(ctx, userKey(userID))

	if len(room.Participants) == 0 || userID == room.CreatorUserID {
		return m.store.deleteRoom(ctx, room)
	}
	return m.store.saveRoom(ctx, room)
}

func (m *MeetingManager) endMeetingLocked(ctx context.Context, roomID, userID string) ([]string, error) {
	room, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	if room.CreatorUserID != userID {
		return nil, fmt.Errorf("only creator can end meeting")
	}
	participantIDs := make([]string, 0, len(room.Participants))
	for uid := range room.Participants {
		participantIDs = append(participantIDs, uid)
	}
	if err := m.store.deleteRoom(ctx, room); err != nil {
		return nil, err
	}
	return participantIDs, nil
}

func abnormalLeavePeerID(room *MeetingRoom, leaverUserID string) string {
	if room == nil {
		return ""
	}
	if leaverUserID == room.CreatorUserID && len(room.InviteeUserIDs) > 0 {
		return room.InviteeUserIDs[0]
	}
	return room.CreatorUserID
}

func (m *MeetingManager) sendHangupSignal(ctx context.Context, room *MeetingRoom, sendUserID, recvUserID string) {
	if m.im == nil || recvUserID == "" || room == nil {
		return
	}

	signaling := map[string]any{
		"action":         "hangup",
		"roomID":         room.RoomID,
		"callType":       room.CallType,
		"inviterUserID":  room.CreatorUserID,
		"inviteeUserIDs": room.InviteeUserIDs,
		"timestamp":      time.Now().UnixMilli(),
	}

	payload, err := json.Marshal(map[string]any{
		"businessID": rtcSignalingBusinessID,
		"data":       signaling,
	})
	if err != nil {
		log.ZWarn(ctx, "sendHangupSignal payload marshal failed", err)
		return
	}

	imToken, err := m.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		log.ZWarn(ctx, "sendHangupSignal admin token failed", err)
		return
	}
	apiCtx := mctx.WithApiToken(ctx, imToken)

	req := &imapi.SendCustomMsgReq{
		SendID:           sendUserID,
		RecvID:           recvUserID,
		SessionType:      singleChatSessionType,
		ContentType:      customMsgContentType,
		SenderPlatformID: 5,
		Content: map[string]any{
			"data":        string(payload),
			"extension":   "",
			"description": "",
		},
	}

	if err := m.im.SendCustomMessage(apiCtx, req); err != nil {
		log.ZWarn(ctx, "sendHangupSignal SendCustomMessage failed", err, "roomID", room.RoomID, "from", sendUserID, "to", recvUserID)
	}
}
