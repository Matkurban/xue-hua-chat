package rtc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"gitee.com/matkurban/openimsdk-tools/log"
)

const (
	conferenceContentType   = int32(127)
	conferenceStatusOngoing = int32(1)
	conferenceStatusEnded   = int32(2)
	conferenceCallTypeAudio = int32(1)
	conferenceCallTypeVideo = int32(2)
	groupChatSessionType    = int32(3)
)

func conferenceCallTypeInt(callType string) int32 {
	if callType == "audio" {
		return conferenceCallTypeAudio
	}
	return conferenceCallTypeVideo
}

func parseConversationTarget(conversationID, fallbackGroupID, selfUserID string) (sessionType int32, recvID, gid string) {
	// Prefer explicit conversationID so share targets are not forced into source group.
	if strings.HasPrefix(conversationID, "sg_") {
		return groupChatSessionType, "", strings.TrimPrefix(conversationID, "sg_")
	}
	if strings.HasPrefix(conversationID, "si_") {
		parts := strings.Split(strings.TrimPrefix(conversationID, "si_"), "_")
		if len(parts) >= 2 {
			a, b := parts[0], parts[1]
			if a == selfUserID {
				return singleChatSessionType, b, ""
			}
			return singleChatSessionType, a, ""
		}
	}
	if fallbackGroupID != "" {
		return groupChatSessionType, "", fallbackGroupID
	}
	if conversationID != "" && conversationID != selfUserID {
		return singleChatSessionType, conversationID, ""
	}
	return 0, "", ""
}

func (m *ConferenceManager) sendConferenceAnnounce(
	ctx context.Context,
	room *ConferenceRoom,
	conversationID string,
	status int32,
	endedAt *int64,
) (*ConferenceAnnounceRef, error) {
	if m.im == nil || room == nil || conversationID == "" {
		return nil, fmt.Errorf("conference announce unavailable")
	}
	// Use AdminUserID so peer resolution matches SendID after host transfer.
	sessionType, recvID, gid := parseConversationTarget(conversationID, room.GroupID, room.AdminUserID)
	if sessionType == 0 {
		log.ZWarn(ctx, "conference announce: cannot resolve conversation", nil, "conversationID", conversationID)
		return nil, fmt.Errorf("cannot resolve conversation %s", conversationID)
	}
	imToken, err := m.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		log.ZWarn(ctx, "conference announce admin token failed", err)
		return nil, err
	}
	apiCtx := mctx.WithApiToken(ctx, imToken)
	hostNick, hostFace := "", ""
	if u, e := m.im.GetUserInfo(apiCtx, room.AdminUserID); e == nil && u != nil {
		hostNick, hostFace = u.Nickname, u.FaceURL
	}
	content := map[string]any{
		"roomId":    room.RoomID,
		"callType":  conferenceCallTypeInt(room.CallType),
		"status":    status,
		"title":     room.Title,
		"adminUser": map[string]any{"userID": room.AdminUserID, "nickname": hostNick, "faceURL": hostFace},
		"groupID":   room.GroupID,
		"startedAt": room.CreateTime * 1000,
	}
	if endedAt != nil {
		content["endedAt"] = *endedAt
	}
	req := &imapi.SendCustomMsgReq{
		SendID:           room.AdminUserID,
		RecvID:           recvID,
		GroupID:          gid,
		SessionType:      sessionType,
		ContentType:      conferenceContentType,
		SenderPlatformID: 5,
		SenderNickname:   hostNick,
		SenderFaceURL:    hostFace,
		Content:          content,
	}
	resp, err := m.im.SendCustomMessageResp(apiCtx, req)
	if err != nil {
		log.ZWarn(ctx, "conference announce send failed", err, "conversationID", conversationID, "roomID", room.RoomID)
		return nil, err
	}
	return &ConferenceAnnounceRef{
		ConversationID: conversationID,
		ClientMsgID:    resp.ClientMsgID,
		ServerMsgID:    resp.ServerMsgID,
	}, nil
}

func (m *ConferenceManager) announceOnCreate(ctx context.Context, room *ConferenceRoom, conversationID string) {
	if conversationID == "" {
		return
	}
	// Client sends the type-127 card; only remember the source conversation here.
	room.SourceConversationID = conversationID
	_ = m.store.SaveRoom(ctx, room)
}

// RegisterConferenceAnnounce records a client-sent announce card so end/update can patch it.
func (m *ConferenceManager) RegisterConferenceAnnounce(
	roomID, hostID, conversationID, clientMsgID, serverMsgID string,
) error {
	ctx := context.Background()
	conversationID = strings.TrimSpace(conversationID)
	clientMsgID = strings.TrimSpace(clientMsgID)
	if conversationID == "" || clientMsgID == "" {
		return fmt.Errorf("conversationID and clientMsgID required")
	}
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		for _, r := range room.AnnounceRefs {
			if r.ConversationID == conversationID && r.ClientMsgID == clientMsgID {
				return nil
			}
		}
		room.AnnounceRefs = append(room.AnnounceRefs, ConferenceAnnounceRef{
			ConversationID: conversationID,
			ClientMsgID:    clientMsgID,
			ServerMsgID:    strings.TrimSpace(serverMsgID),
		})
		if room.SourceConversationID == "" {
			room.SourceConversationID = conversationID
		}
		room.Rev++
		return m.store.SaveRoom(lockCtx, room)
	})
}

func (m *ConferenceManager) ShareConference(roomID, hostID string, conversationIDs []string) (int, error) {
	ctx := context.Background()
	var sent int
	err := m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		attempted := 0
		for _, cid := range conversationIDs {
			cid = strings.TrimSpace(cid)
			if cid == "" {
				continue
			}
			attempted++
			ref, sendErr := m.sendConferenceAnnounce(lockCtx, room, cid, conferenceStatusOngoing, nil)
			if sendErr != nil || ref == nil {
				continue
			}
			room.AnnounceRefs = append(room.AnnounceRefs, *ref)
			sent++
		}
		if attempted > 0 && sent == 0 {
			return fmt.Errorf("share failed: no announce delivered")
		}
		room.Rev++
		return m.store.SaveRoom(lockCtx, room)
	})
	return sent, err
}

func (m *ConferenceManager) updateAnnounceMsgsEnded(ctx context.Context, room *ConferenceRoom, endedAt int64) {
	if room == nil {
		return
	}
	if m.im == nil {
		log.ZWarn(ctx, "conference announce UpdateMsg skipped: im nil", nil, "roomID", room.RoomID)
		return
	}
	if len(room.AnnounceRefs) == 0 {
		log.ZWarn(ctx, "conference announce UpdateMsg skipped: empty AnnounceRefs", nil,
			"roomID", room.RoomID, "sourceConversationID", room.SourceConversationID)
		return
	}
	imToken, err := m.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		log.ZWarn(ctx, "conference announce UpdateMsg admin token failed", err)
		return
	}
	apiCtx := mctx.WithApiToken(ctx, imToken)
	merge := map[string]any{
		"status":  conferenceStatusEnded,
		"endedAt": endedAt,
	}
	for _, ref := range room.AnnounceRefs {
		if ref.ConversationID == "" || ref.ClientMsgID == "" {
			continue
		}
		if err := m.im.ModifyMsgContent(apiCtx, ref.ConversationID, ref.ClientMsgID, merge); err != nil {
			log.ZWarn(ctx, "conference announce UpdateMsg failed", err,
				"conversationID", ref.ConversationID, "clientMsgID", ref.ClientMsgID, "roomID", room.RoomID)
		}
	}
}

func (m *ConferenceManager) conversationMemberIDs(ctx context.Context, conversationID string, room *ConferenceRoom) []string {
	if conversationID == "" || room == nil {
		return nil
	}
	if strings.HasPrefix(conversationID, "si_") {
		parts := strings.Split(strings.TrimPrefix(conversationID, "si_"), "_")
		if len(parts) >= 2 {
			out := make([]string, 0, 2)
			for _, p := range parts[:2] {
				if p != "" {
					out = append(out, p)
				}
			}
			return out
		}
	}
	sessionType, recvID, gid := parseConversationTarget(conversationID, room.GroupID, room.AdminUserID)
	if sessionType == groupChatSessionType && gid != "" {
		if m.im == nil {
			return nil
		}
		imToken, err := m.im.ImAdminTokenWithDefaultAdmin(ctx)
		if err != nil {
			log.ZWarn(ctx, "conference announce get group members token failed", err)
			return nil
		}
		apiCtx := mctx.WithApiToken(ctx, imToken)
		ids, err := m.im.GetGroupMemberUserIDs(apiCtx, gid)
		if err != nil {
			log.ZWarn(ctx, "conference announce get group members failed", err, "groupID", gid)
			return nil
		}
		return ids
	}
	if sessionType == singleChatSessionType && recvID != "" {
		out := []string{recvID}
		if room.AdminUserID != "" && room.AdminUserID != recvID {
			out = append(out, room.AdminUserID)
		}
		if room.CreatorUserID != "" && room.CreatorUserID != recvID && room.CreatorUserID != room.AdminUserID {
			out = append(out, room.CreatorUserID)
		}
		return out
	}
	return nil
}

func (m *ConferenceManager) announceRecipientIDs(ctx context.Context, room *ConferenceRoom) []string {
	ids, _ := m.store.AllMemberIDs(ctx, room.RoomID)
	seen := make(map[string]struct{}, len(ids)+8)
	for _, id := range ids {
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, ref := range room.AnnounceRefs {
		for _, uid := range m.conversationMemberIDs(ctx, ref.ConversationID, room) {
			if uid == "" {
				continue
			}
			if _, ok := seen[uid]; ok {
				continue
			}
			seen[uid] = struct{}{}
			ids = append(ids, uid)
		}
	}
	return ids
}

func (m *ConferenceManager) broadcastAnnounceEnded(ctx context.Context, room *ConferenceRoom, endedAt int64, extraUserIDs ...string) {
	if room == nil {
		return
	}
	refsJSON, _ := json.Marshal(room.AnnounceRefs)
	payload := &ConferenceSyncPayload{
		Event:            "announce_updated",
		RoomID:           room.RoomID,
		Rev:              room.Rev,
		Status:           conferenceStatusEnded,
		EndedAt:          endedAt,
		AnnounceRefsJSON: string(refsJSON),
	}
	recipients := mergeUniqueUserIDs(m.announceRecipientIDs(ctx, room), extraUserIDs...)
	m.broadcastSync(ctx, recipients, payload)
	// Mongo history write is slow; keep it off the end path so DeleteRoomAll /
	// LiveKit teardown (and peer exit) are not delayed.
	roomSnap := *room
	roomSnap.AnnounceRefs = append([]ConferenceAnnounceRef(nil), room.AnnounceRefs...)
	go m.updateAnnounceMsgsEnded(context.Background(), &roomSnap, endedAt)
}
