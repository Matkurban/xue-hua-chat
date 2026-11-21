package rtc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"chat/pkg/common/mctx"
	"chat/pkg/common/log"
	constantpb "github.com/openimsdk/protocol/constant"
)

const (
	conferenceSyncBusinessID    = "conference_sync"
	conferenceInviteBusinessID  = "conference_invite"
	conferenceStartedBusinessID = "conference_started"
	conferenceClosedBusinessID  = "conference_closed"
)

// ctxWithOperationID ensures ctx carries a non-empty operationID for IM HTTP APIs.
// mw.GinParseOperationID rejects POST requests without this header.
func ctxWithOperationID(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if v, ok := ctx.Value(constantpb.OperationID).(string); ok && v != "" {
		return ctx
	}
	return context.WithValue(ctx, constantpb.OperationID, fmt.Sprintf("conf-sync-%d", time.Now().UnixNano()))
}

// ConferenceSyncPayload is fan-out state for conference_sync (proto3 binary on the wire).
type ConferenceSyncPayload struct {
	RoomID               string `json:"roomID"`
	Rev                  int64  `json:"rev"`
	AdminUserID          string `json:"adminUserID"`
	ParticipantCount     int    `json:"participantCount"`
	GlobalAudioMuted     bool   `json:"globalAudioMuted"`
	GlobalVideoOff       bool   `json:"globalVideoOff"`
	ChatMuted            bool   `json:"chatMuted"`
	Locked               bool   `json:"locked"`
	PinnedUserID         string `json:"pinnedUserID,omitempty"`
	EgressID             string `json:"egressID,omitempty"`
	IngressID            string `json:"ingressID,omitempty"`
	Event                string `json:"event"`
	TargetUserID         string `json:"targetUserID,omitempty"`
	TargetAudioForcedOff bool   `json:"targetAudioForcedOff,omitempty"`
	TargetVideoForcedOff bool   `json:"targetVideoForcedOff,omitempty"`
	TargetChatForcedOff  bool   `json:"targetChatForcedOff,omitempty"`
	// announce_updated
	Status           int32  `json:"status,omitempty"`
	EndedAt          int64  `json:"endedAt,omitempty"`
	AnnounceRefsJSON string `json:"announceRefsJson,omitempty"`
	// stage / camera snapshot JSON (ConferenceStageState)
	StageStateJSON string `json:"stageStateJson,omitempty"`
	// chat_revoke: TargetUserID = original sender; RevokeTargetSeq = message seq
	RevokeTargetSeq uint64 `json:"revokeTargetSeq,omitempty"`
}

func BuildConferenceSync(room *ConferenceRoom, event, target string) *ConferenceSyncPayload {
	if room == nil {
		return &ConferenceSyncPayload{Event: event, TargetUserID: target}
	}
	return &ConferenceSyncPayload{
		RoomID: room.RoomID, Rev: room.Rev, AdminUserID: room.AdminUserID,
		ParticipantCount: room.ParticipantCount,
		GlobalAudioMuted: room.GlobalAudioMuted, GlobalVideoOff: room.GlobalVideoOff,
		ChatMuted: room.ChatMuted, Locked: room.Locked, PinnedUserID: room.PinnedUserID,
		EgressID: room.EgressID, IngressID: room.IngressID,
		Event: event, TargetUserID: target,
	}
}

func (m *ConferenceManager) broadcastEnded(_ context.Context, roomID string, userIDs []string) {
	// Detach from caller ctx so ended delivery is not cancelled mid fan-out.
	notifyCtx := context.Background()
	attempted, failed := m.broadcastSync(notifyCtx, userIDs, &ConferenceSyncPayload{RoomID: roomID, Event: "ended"})
	if attempted == 0 {
		log.ZWarn(notifyCtx, "conference_sync ended notify skipped: no recipients", nil, "roomID", roomID)
		return
	}
	if failed == attempted {
		log.ZError(notifyCtx, "conference_sync ended notify all failed", nil,
			"roomID", roomID, "recipients", attempted)
		return
	}
	if failed > 0 {
		log.ZWarn(notifyCtx, "conference_sync ended notify partial failure", nil,
			"roomID", roomID, "failed", failed, "recipients", attempted)
	}
}

func (m *ConferenceManager) memberIDsForSync(ctx context.Context, roomID string, extraUserIDs ...string) []string {
	ids, _ := m.store.AllMemberIDs(ctx, roomID)
	if len(extraUserIDs) == 0 {
		return ids
	}
	seen := make(map[string]struct{}, len(ids)+len(extraUserIDs))
	for _, id := range ids {
		seen[id] = struct{}{}
	}
	for _, id := range extraUserIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			ids = append(ids, id)
			seen[id] = struct{}{}
		}
	}
	return ids
}

func (m *ConferenceManager) notifyRoomSync(ctx context.Context, room *ConferenceRoom, event, target string, extraUserIDs ...string) {
	if room == nil {
		return
	}
	payload := BuildConferenceSync(room, event, target)
	m.attachStageState(ctx, room, payload, nil)
	m.broadcastSync(ctx, m.memberIDsForSync(ctx, room.RoomID, extraUserIDs...), payload)
}

func (m *ConferenceManager) bumpAndSync(ctx context.Context, room *ConferenceRoom, event, target string, extraUserIDs ...string) error {
	if err := m.store.BumpRev(ctx, room); err != nil {
		return err
	}
	m.notifyRoomSync(ctx, room, event, target, extraUserIDs...)
	return nil
}

// bumpAndSyncPayload bumps rev on the in-memory room (must already hold latest policy fields),
// then fans out the provided payload (e.g. media force flags).
func (m *ConferenceManager) bumpAndSyncPayload(ctx context.Context, room *ConferenceRoom, payload *ConferenceSyncPayload, extraUserIDs ...string) error {
	_, _, err := m.bumpAndSyncPayloadReport(ctx, room, payload, extraUserIDs...)
	return err
}

// bumpAndSyncPayloadReport is like bumpAndSyncPayload but returns fan-out stats.
func (m *ConferenceManager) bumpAndSyncPayloadReport(ctx context.Context, room *ConferenceRoom, payload *ConferenceSyncPayload, extraUserIDs ...string) (attempted, failed int, err error) {
	if room == nil || payload == nil {
		return 0, 0, nil
	}
	if err := m.store.BumpRev(ctx, room); err != nil {
		return 0, 0, err
	}
	payload.RoomID = room.RoomID
	payload.Rev = room.Rev
	payload.ParticipantCount = room.ParticipantCount
	payload.AdminUserID = room.AdminUserID
	payload.GlobalAudioMuted = room.GlobalAudioMuted
	payload.GlobalVideoOff = room.GlobalVideoOff
	payload.ChatMuted = room.ChatMuted
	payload.Locked = room.Locked
	payload.PinnedUserID = room.PinnedUserID
	payload.EgressID = room.EgressID
	payload.IngressID = room.IngressID
	if payload.StageStateJSON == "" {
		m.attachStageState(ctx, room, payload, nil)
	}
	attempted, failed = m.broadcastSync(ctx, m.memberIDsForSync(ctx, room.RoomID, extraUserIDs...), payload)
	if attempted > 0 && failed == attempted {
		log.ZError(ctx, "conference_sync notify all failed", nil,
			"event", payload.Event, "roomID", payload.RoomID, "recipients", attempted)
	}
	return attempted, failed, nil
}

// broadcastSync fans out conference_sync to userIDs.
// Returns attempted recipient count and how many sends failed (including total failure when IM is unavailable).
func (m *ConferenceManager) broadcastSync(ctx context.Context, userIDs []string, payload *ConferenceSyncPayload) (attempted, failed int) {
	if payload == nil || len(userIDs) == 0 {
		return 0, 0
	}
	for _, uid := range userIDs {
		if uid != "" {
			attempted++
		}
	}
	if attempted == 0 {
		return 0, 0
	}
	ctx = ctxWithOperationID(ctx)
	if m.im == nil {
		log.ZError(ctx, "conference_sync skipped: im client nil", nil,
			"event", payload.Event, "roomID", payload.RoomID, "recipients", attempted)
		return attempted, attempted
	}
	protoBytes := MarshalConferenceSyncProto(payload)
	dataB64 := base64.StdEncoding.EncodeToString(protoBytes)
	imToken, err := m.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		log.ZError(ctx, "conference_sync admin token failed", err,
			"event", payload.Event, "roomID", payload.RoomID, "recipients", attempted)
		return attempted, attempted
	}
	apiCtx := mctx.WithApiToken(ctx, imToken)
	const concurrency = 16
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var failedCount atomic.Int64
	for _, uid := range userIDs {
		if uid == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(uid string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := m.im.SendBusinessNotification(apiCtx, conferenceSyncBusinessID, dataB64, m.adminUserID, uid); err != nil {
				failedCount.Add(1)
				log.ZWarn(ctx, "conference_sync notify failed", err, "userID", uid, "event", payload.Event)
			}
		}(uid)
	}
	wg.Wait()
	return attempted, int(failedCount.Load())
}

func (m *ConferenceManager) sendConferenceInvites(ctx context.Context, room *ConferenceRoom, inviterUserID string, inviteeUserIDs []string) {
	if m.im == nil || room == nil || len(inviteeUserIDs) == 0 {
		return
	}
	ctx = ctxWithOperationID(ctx)
	payload := map[string]any{
		"roomID":         room.RoomID,
		"callType":       room.CallType,
		"adminUserID":    room.AdminUserID,
		"inviterUserID":  inviterUserID,
		"inviteeUserIDs": room.InviteeUserIDs,
		"groupID":        room.GroupID,
		"timestamp":      time.Now().UnixMilli(),
	}
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}
	imToken, err := m.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		log.ZWarn(ctx, "conference_invite admin token failed", err)
		return
	}
	apiCtx := mctx.WithApiToken(ctx, imToken)
	const concurrency = 16
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, uid := range inviteeUserIDs {
		if uid == "" || uid == inviterUserID {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(uid string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := m.im.SendBusinessNotification(apiCtx, conferenceInviteBusinessID, string(dataBytes), inviterUserID, uid); err != nil {
				log.ZWarn(ctx, "conference_invite business notify failed", err, "userID", uid)
			}
		}(uid)
	}
	wg.Wait()
}

func (m *ConferenceManager) notifyConferenceStarted(ctx context.Context, room *ConferenceRoom) {
	if m == nil || room == nil {
		return
	}
	payload := PublicActiveConferenceFromRoom(room, 0)
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	known := m.lifecycleKnownUserIDs(ctx, room)
	m.sendLifecycleNotifications(ctx, conferenceStartedBusinessID, string(data), known)
	m.fanoutLifecycleAsync(conferenceStartedBusinessID, string(data), known)
}

func (m *ConferenceManager) notifyConferenceClosed(ctx context.Context, room *ConferenceRoom, extraUserIDs ...string) {
	if m == nil || room == nil || room.RoomID == "" {
		return
	}
	data, err := json.Marshal(map[string]string{"roomID": room.RoomID})
	if err != nil {
		return
	}
	known := mergeUniqueUserIDs(m.lifecycleKnownUserIDs(ctx, room), extraUserIDs...)
	m.sendLifecycleNotifications(ctx, conferenceClosedBusinessID, string(data), known)
	m.fanoutLifecycleAsync(conferenceClosedBusinessID, string(data), known)
}

func (m *ConferenceManager) lifecycleKnownUserIDs(ctx context.Context, room *ConferenceRoom) []string {
	if room == nil {
		return nil
	}
	ids, _ := m.store.AllMemberIDs(ctx, room.RoomID)
	return mergeUniqueUserIDs(ids, append([]string{room.CreatorUserID, room.AdminUserID}, room.InviteeUserIDs...)...)
}

func (m *ConferenceManager) sendLifecycleNotifications(ctx context.Context, key, data string, userIDs []string) {
	if m.im == nil || key == "" || len(userIDs) == 0 {
		return
	}
	ctx = ctxWithOperationID(ctx)
	imToken, err := m.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		log.ZWarn(ctx, "conference lifecycle admin token failed", err, "key", key)
		return
	}
	apiCtx := mctx.WithApiToken(ctx, imToken)
	sendID := m.adminUserID
	if sendID == "" {
		sendID = "imAdmin"
	}
	const concurrency = 8
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, uid := range userIDs {
		if uid == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(uid string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := m.im.SendBusinessNotification(apiCtx, key, data, sendID, uid); err != nil {
				log.ZWarn(ctx, "conference lifecycle notify failed", err, "key", key, "userID", uid)
			}
		}(uid)
	}
	wg.Wait()
}

func (m *ConferenceManager) fanoutLifecycleAsync(key, data string, alreadySent []string) {
	lister := m.lifecycleRecipients
	if lister == nil {
		return
	}
	sent := make(map[string]struct{}, len(alreadySent))
	for _, id := range alreadySent {
		if id != "" {
			sent[id] = struct{}{}
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(ctxWithOperationID(context.Background()), 45*time.Second)
		defer cancel()
		extra := lister(ctx)
		if len(extra) == 0 {
			return
		}
		var pending []string
		for _, id := range extra {
			if id == "" {
				continue
			}
			if _, ok := sent[id]; ok {
				continue
			}
			sent[id] = struct{}{}
			pending = append(pending, id)
		}
		m.sendLifecycleNotifications(ctx, key, data, pending)
	}()
}

// PickLifecycleRecipients prefers IM online users. If that lookup fails, it
// falls back to fallbackFn (typically a paginated chat-user scan). An empty
// online list is a successful result and must not trigger the fallback.
func PickLifecycleRecipients(
	ctx context.Context,
	onlineFn func(context.Context) ([]string, error),
	fallbackFn func(context.Context) []string,
) []string {
	if onlineFn != nil {
		callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		ids, err := onlineFn(callCtx)
		cancel()
		if err == nil {
			return ids
		}
		log.ZWarn(ctx, "conference lifecycle online users failed, falling back to chat user scan", err)
	}
	if fallbackFn != nil {
		return fallbackFn(ctx)
	}
	return nil
}
