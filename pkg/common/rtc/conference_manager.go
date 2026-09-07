package rtc

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"gitee.com/matkurban/openimsdk-tools/log"
	"github.com/livekit/protocol/livekit"
	"github.com/redis/go-redis/v9"
)

const (
	conferenceRoomIDMaxAttempts = 16
	conferenceRoomIDMod         = 1_000_000_000 // 0 … 999_999_999 → "%09d"
)

// generateConferenceRoomID returns a fixed-length 9-digit decimal string (leading zeros kept).
func generateConferenceRoomID() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(conferenceRoomIDMod))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%09d", n.Int64()), nil
}

// allocateConferenceRoomID picks a free 9-digit room ID (not already in Redis).
func (m *ConferenceManager) allocateConferenceRoomID(ctx context.Context) (string, error) {
	for i := 0; i < conferenceRoomIDMaxAttempts; i++ {
		id, err := generateConferenceRoomID()
		if err != nil {
			return "", err
		}
		_, ok, err := m.store.GetRoom(ctx, id)
		if err != nil {
			return "", err
		}
		if !ok {
			return id, nil
		}
	}
	return "", fmt.Errorf("failed to allocate unique conference room ID after %d attempts", conferenceRoomIDMaxAttempts)
}

// ConferenceManager manages large multi-role conferences (independent of MeetingManager).
type ConferenceManager struct {
	store          *RedisConferenceStore
	lk             conferenceLiveKit
	im             imapi.CallerInterface
	adminUserID    string
	autoStageLimit int
	eventCh        chan *livekit.WebhookEvent
	stopCh         chan struct{}
	wg             sync.WaitGroup
	onRoomEnded    func(roomID, occurrenceID string)
	waiting        *WaitingRoomStore
	pwdLimiter     *PasswordRateLimiter
	// lifecycleRecipients, when set, supplies extra user IDs (typically all
	// registered users) for conference_started / conference_closed fan-out.
	lifecycleRecipients LifecycleRecipientLister
}

// LifecycleRecipientLister returns extra recipients for global conference lifecycle
// notifications. Must not block for long; pagination belongs inside the callback.
type LifecycleRecipientLister func(ctx context.Context) []string

// SetWaitingRoomStore 注入独立等候室。
func (m *ConferenceManager) SetWaitingRoomStore(w *WaitingRoomStore) {
	m.waiting = w
}

// SetPasswordLimiter 注入密码限流。
func (m *ConferenceManager) SetPasswordLimiter(l *PasswordRateLimiter) {
	m.pwdLimiter = l
}

// SetLifecycleRecipientLister injects the global recipient source for started/closed
// business notifications. Nil is valid (known members/invitees still receive them).
func (m *ConferenceManager) SetLifecycleRecipientLister(fn LifecycleRecipientLister) {
	if m == nil {
		return
	}
	m.lifecycleRecipients = fn
}

func NewConferenceManager(
	rdb redis.UniversalClient,
	key, secret, url string,
	im imapi.CallerInterface,
	adminUserID string,
	autoStageLimit int,
) (*ConferenceManager, error) {
	m, err := newConferenceManagerCore(rdb, key, secret, url, im, adminUserID, autoStageLimit)
	if err != nil {
		return nil, err
	}
	m.wg.Add(3)
	go m.cleanupLoop()
	go m.webhookWorker()
	go m.pendingLeaveLoop()
	return m, nil
}

func newConferenceManagerCore(
	rdb redis.UniversalClient,
	key, secret, url string,
	im imapi.CallerInterface,
	adminUserID string,
	autoStageLimit int,
) (*ConferenceManager, error) {
	lk, err := newConferenceLK(url, key, secret)
	if err != nil {
		return nil, err
	}
	return &ConferenceManager{
		store:          NewRedisConferenceStore(rdb),
		lk:             lk,
		im:             im,
		adminUserID:    adminUserID,
		autoStageLimit: normalizeStageLimit(autoStageLimit, DefaultAutoStageLimit),
		eventCh:        make(chan *livekit.WebhookEvent, 256),
		stopCh:         make(chan struct{}),
	}, nil
}

func (m *ConferenceManager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

func (m *ConferenceManager) GetLiveKitURL() string { return m.lk.ClientURL() }
func (m *ConferenceManager) LiveKitKey() string    { return m.lk.APIKey() }
func (m *ConferenceManager) LiveKitSecret() string { return m.lk.APISecret() }

func extractConversationID(inviteeUserIDs []string, explicit string) (cleaned []string, conversationID string) {
	conversationID = explicit
	for _, id := range inviteeUserIDs {
		if strings.HasPrefix(id, "conversation:") {
			cid := strings.TrimPrefix(id, "conversation:")
			if conversationID == "" {
				conversationID = cid
			}
			continue
		}
		cleaned = append(cleaned, id)
	}
	return cleaned, conversationID
}

func (m *ConferenceManager) CreateConference(
	creatorUserID, callType, title string, inviteeUserIDs []string, groupID, conversationID string,
) (*ConferenceRoom, string, error) {
	return m.CreateConferenceOpts(CreateConferenceOpts{
		CreatorUserID:  creatorUserID,
		CallType:       callType,
		Title:          title,
		InviteeUserIDs: inviteeUserIDs,
		GroupID:        groupID,
		ConversationID: conversationID,
	})
}

// CreateConferenceOpts 创建大会可选参数（含预约预分配会议号）。
type CreateConferenceOpts struct {
	RoomID               string
	CreatorUserID        string
	CallType             string
	Title                string
	GroupID              string
	ConversationID       string
	PasswordHash         string
	ScheduleOccurrenceID string
	InviteeUserIDs       []string
	WaitingRoomEnabled   bool
	MuteOnJoin           bool
	Locked               bool
	SkipInvites          bool
	SkipAnnounce         bool
	SkipAddCreator       bool // 仅确保房间存在，不把创建者写入成员（成员走 Join）
}

func (m *ConferenceManager) CreateConferenceOpts(opts CreateConferenceOpts) (*ConferenceRoom, string, error) {
	ctx := context.Background()
	roomID := opts.RoomID
	if roomID == "" {
		var err error
		roomID, err = m.allocateConferenceRoomID(ctx)
		if err != nil {
			return nil, "", err
		}
	} else {
		if existing, ok, err := m.store.GetRoom(ctx, roomID); err != nil {
			return nil, "", err
		} else if ok && existing != nil {
			// already activated
			tok := ""
			if !opts.SkipAddCreator {
				var err error
				tok, err = m.lk.GenerateToken(roomID, opts.CreatorUserID, ConferenceRoleAdmin, true, true, true)
				if err != nil {
					return nil, "", err
				}
			}
			return existing, tok, nil
		}
	}
	now := time.Now().Unix()
	inviteeUserIDs, conversationID := extractConversationID(opts.InviteeUserIDs, opts.ConversationID)
	invitees := normalizeCreateInvitees(opts.CreatorUserID, inviteeUserIDs)
	room := &ConferenceRoom{
		RoomID:               roomID,
		AdminUserID:          opts.CreatorUserID,
		CreatorUserID:        opts.CreatorUserID,
		CallType:             opts.CallType,
		Title:                opts.Title,
		InviteeUserIDs:       invitees,
		GroupID:              opts.GroupID,
		SourceConversationID: conversationID,
		CreateTime:           now,
		ParticipantCount:     0,
		Rev:                  1,
		AutoStageLimit:       m.autoStageLimit,
		PasswordHash:         opts.PasswordHash,
		WaitingRoomEnabled:   opts.WaitingRoomEnabled,
		ScheduleOccurrenceID: opts.ScheduleOccurrenceID,
		GlobalAudioMuted:     opts.MuteOnJoin,
		// Waiting room is independent of Locked; admitted users must still join.
		Locked: opts.Locked,
	}
	if err := m.lk.CreateRoom(ctx, roomID, ConferenceMaxParticipants); err != nil {
		log.ZWarn(ctx, "CreateRoom failed (may already exist)", err, "roomID", roomID)
	}
	token := ""
	if !opts.SkipAddCreator {
		var err error
		token, err = m.lk.GenerateToken(roomID, opts.CreatorUserID, ConferenceRoleAdmin, true, true, true)
		if err != nil {
			return nil, "", err
		}
	}
	if err := m.store.SaveRoom(ctx, room); err != nil {
		return nil, "", err
	}
	if !opts.SkipAddCreator {
		member := &ConferenceMember{
			UserID: opts.CreatorUserID, Role: ConferenceRoleAdmin, JoinTime: now, OnStage: true,
		}
		m.fillMemberProfile(ctx, member)
		if err := m.store.SaveMember(ctx, roomID, member); err != nil {
			_ = m.store.DeleteRoomAll(ctx, roomID)
			return nil, "", err
		}
		if err := m.store.SetUserRoom(ctx, opts.CreatorUserID, roomID); err != nil {
			_ = m.store.DeleteRoomAll(ctx, roomID)
			return nil, "", err
		}
		room.ParticipantCount = 1
		_ = m.store.SaveRoom(ctx, room)
		if !opts.SkipInvites {
			m.sendConferenceInvites(ctx, room, opts.CreatorUserID, invitees)
		}
		if !opts.SkipAnnounce {
			m.announceOnCreate(ctx, room, conversationID)
		}
	}
	m.notifyConferenceStarted(ctx, room)
	return room, token, nil
}

// RoomExists 检查 Redis 中是否已有房间。
func (m *ConferenceManager) RoomExists(roomID string) bool {
	_, ok, err := m.store.GetRoom(context.Background(), roomID)
	return err == nil && ok
}

// ConferenceStore 暴露底层 store（等候室 deny 等）。
func (m *ConferenceManager) ConferenceStore() *RedisConferenceStore {
	if m == nil {
		return nil
	}
	return m.store
}

// SetOnRoomEnded 注册房间结束回调（预约场次联动 ended）。只保存一个回调。
func (m *ConferenceManager) SetOnRoomEnded(fn func(roomID, occurrenceID string)) {
	m.onRoomEnded = fn
}

// normalizeCreateInvitees dedupes invitees and drops the creator.
func normalizeCreateInvitees(creatorUserID string, inviteeUserIDs []string) []string {
	seen := map[string]struct{}{}
	var invitees []string
	for _, id := range inviteeUserIDs {
		if id == "" || id == creatorUserID {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		invitees = append(invitees, id)
	}
	return invitees
}

// memberIsModerator reports whether mem is the room host or a co-host.
func memberIsModerator(room *ConferenceRoom, mem *ConferenceMember) bool {
	if room == nil || mem == nil {
		return false
	}
	if mem.UserID == room.AdminUserID {
		return true
	}
	return mem.Role == ConferenceRoleCohost
}

func mediaAllowFlags(room *ConferenceRoom, mem *ConferenceMember) (allowMic, allowCamera bool) {
	if mem == nil || !mem.OnStage {
		return false, false
	}
	allowMic, allowCamera = true, true
	if room != nil {
		// Host / co-host may unmute themselves during mute-all; members stay blocked.
		if room.GlobalAudioMuted && !memberIsModerator(room, mem) {
			allowMic = false
		}
		if room.GlobalVideoOff && !memberIsModerator(room, mem) {
			allowCamera = false
		}
	}
	if mem.AudioForcedOff {
		allowMic = false
	}
	if mem.VideoForcedOff {
		allowCamera = false
	}
	return allowMic, allowCamera
}

// memberCanPublishData：房主/联席始终可发；否则全员文字禁言或个人文字禁言任一生效则不可发。
func memberCanPublishData(room *ConferenceRoom, mem *ConferenceMember) bool {
	if room == nil || mem == nil {
		return false
	}
	if memberIsModerator(room, mem) {
		return true
	}
	return !room.ChatMuted && !mem.ChatForcedOff
}

// forceMuteMediaKind mutes a member's audio or video at the SFU.
//
// LiveKit refuses remote unmute unless room.enable_remote_unmute is on, and
// re-opening someone's mic without consent is not the intended semantics: lifting
// a restriction only restores the permission, the member re-enables the device.
func (m *ConferenceManager) forceMuteMediaKind(ctx context.Context, roomID, userID string, video bool) {
	if err := m.lk.MuteMediaKind(ctx, roomID, userID, video, true); err != nil {
		log.ZWarn(ctx, "conference mute track failed", err,
			"roomID", roomID, "userID", userID, "video", video)
	}
}

func (m *ConferenceManager) syncParticipantPublishPerm(
	ctx context.Context, room *ConferenceRoom, mem *ConferenceMember, canPublishData bool,
) {
	if room == nil || mem == nil {
		return
	}
	allowMic, allowCam := mediaAllowFlags(room, mem)
	// LiveKit treats empty CanPublishSources as "all sources"; when both are
	// forced off, revoke CanPublish instead of sending an empty allow-list.
	canPublish := allowMic || allowCam
	_ = m.lk.UpdateParticipantPermission(
		ctx, room.RoomID, mem.UserID,
		canPublish, canPublishData, mem.UserID == room.AdminUserID, allowMic, allowCam,
		mem,
	)
}

func (m *ConferenceManager) allowedJoin(room *ConferenceRoom, userID string) bool {
	if userID == room.CreatorUserID || userID == room.AdminUserID {
		return true
	}
	for _, id := range room.InviteeUserIDs {
		if id == userID {
			return true
		}
	}
	// Unlocked rooms: roomId is the join credential (share / copy ID).
	if !room.Locked {
		return true
	}
	return false
}

func (m *ConferenceManager) JoinConference(roomID, userID string) (string, *ConferenceRoom, *ConferenceMember, error) {
	ctx := context.Background()
	var token string
	var room *ConferenceRoom
	var member *ConferenceMember
	err := m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		r, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("conference %s not found", roomID)
		}
		denied, err := m.store.IsDenied(lockCtx, roomID, userID)
		if err != nil {
			return err
		}
		if denied {
			return fmt.Errorf("user denied from conference")
		}
		if err := m.ensureRoomStageDefaults(lockCtx, r); err != nil {
			return err
		}
		if existing, exists, _ := m.store.GetMember(lockCtx, roomID, userID); exists {
			// Rejoin allowed even when locked (already a member).
			if (userID == r.AdminUserID || existing.Role == ConferenceRoleAdmin) && !existing.OnStage {
				existing.OnStage = true
				existing.SpeakPending = false
				_ = m.store.SaveMember(lockCtx, roomID, existing)
			}
			canData := memberCanPublishData(r, existing)
			allowMic, allowCam := mediaAllowFlags(r, existing)
			tok, err := m.lk.GenerateToken(roomID, userID, existing.Role, canData, allowMic, allowCam)
			if err != nil {
				return err
			}
			// Offline reconnect: give LiveKit connect time before grace leave fires.
			if !existing.InLiveKit {
				due := time.Now().Add(conferenceDisconnectGrace).UnixMilli()
				_ = m.store.SchedulePendingLeave(lockCtx, roomID, userID, due)
			}
			token, room, member = tok, r, existing
			return m.store.SetUserRoom(lockCtx, userID, roomID)
		}
		// Locked: host/invitee via allowedJoin; unlocked: anyone with roomId.
		// Waiting-room admit also grants join even when locked / not invitee.
		admitted := false
		if m.waiting != nil {
			admitted, _ = m.waiting.IsAdmitted(lockCtx, roomID, userID)
		}
		if !m.allowedJoin(r, userID) && !admitted {
			if r.Locked {
				return fmt.Errorf("conference is locked")
			}
			return fmt.Errorf("not invited to conference")
		}
		n, err := m.store.MemberCount(lockCtx, roomID)
		if err != nil {
			return err
		}
		if n >= ConferenceMaxParticipants {
			return fmt.Errorf("conference full")
		}
		role := ConferenceRoleUser
		if userID == r.AdminUserID || userID == r.CreatorUserID {
			role = ConferenceRoleAdmin
		}
		mem := &ConferenceMember{
			UserID: userID, Role: role, JoinTime: time.Now().Unix(),
		}
		if role == ConferenceRoleAdmin {
			mem.OnStage = true
		}
		m.fillMemberProfile(lockCtx, mem)
		canData := memberCanPublishData(r, mem)
		allowMic, allowCam := mediaAllowFlags(r, mem)
		tok, err := m.lk.GenerateToken(roomID, userID, mem.Role, canData, allowMic, allowCam)
		if err != nil {
			return err
		}
		if err := m.store.SaveMember(lockCtx, roomID, mem); err != nil {
			return err
		}
		if err := m.store.SetUserRoom(lockCtx, userID, roomID); err != nil {
			return err
		}
		n2, _ := m.store.MemberCount(lockCtx, roomID)
		r.RefreshParticipantCount(int(n2))
		payload := BuildConferenceSync(r, "joined", userID)
		m.attachStageState(lockCtx, r, payload, mem)
		if err := m.bumpAndSyncPayload(lockCtx, r, payload); err != nil {
			return err
		}
		token, room, member = tok, r, mem
		return nil
	})
	return token, room, member, err
}

// LeaveConference removes userID from the room. ended is true when this leave
// (or host auto-transfer fallback) destroys the room.
func (m *ConferenceManager) LeaveConference(roomID, userID string) (ended bool, err error) {
	ctx := context.Background()
	var roomEnded bool
	err = m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok || room == nil {
			// Already gone — treat as ended so clients can flip announce cards.
			roomEnded = true
			return nil
		}
		wasAdmin := room.AdminUserID == userID
		// Snapshot before RemoveMember so end fan-out still reaches the leaver.
		preIDs, _ := m.store.AllMemberIDs(lockCtx, roomID)
		_ = m.lk.RemoveParticipant(lockCtx, roomID, userID)
		m.clearMemberStageOnLeave(lockCtx, room, userID)
		if err := m.store.RemoveMember(lockCtx, roomID, userID); err != nil {
			return err
		}
		n, _ := m.store.MemberCount(lockCtx, roomID)
		if n == 0 {
			roomEnded = true
			return m.endLocked(lockCtx, room, preIDs...)
		}
		if wasAdmin {
			if err := m.autoTransferOrEnd(lockCtx, room, userID); err != nil {
				return err
			}
			_, stillOk, _ := m.store.GetRoom(lockCtx, roomID)
			roomEnded = !stillOk
			return nil
		}
		room.RefreshParticipantCount(int(n))
		_ = m.store.SaveRoom(lockCtx, room)
		payload := BuildConferenceSync(room, "left", userID)
		m.attachStageState(lockCtx, room, payload, nil)
		return m.bumpAndSyncPayload(lockCtx, room, payload)
	})
	return roomEnded, err
}

func (m *ConferenceManager) autoTransferOrEnd(ctx context.Context, room *ConferenceRoom, leaveUserID string) error {
	ids, err := m.store.AllMemberIDs(ctx, room.RoomID)
	if err != nil {
		return err
	}
	var earliestCohost, earliest *ConferenceMember
	for _, id := range ids {
		mem, ok, _ := m.store.GetMember(ctx, room.RoomID, id)
		if !ok {
			continue
		}
		if mem.Role == ConferenceRoleCohost {
			if earliestCohost == nil || mem.JoinTime < earliestCohost.JoinTime {
				earliestCohost = mem
			}
		}
		if earliest == nil || mem.JoinTime < earliest.JoinTime {
			earliest = mem
		}
	}
	target := earliest
	if earliestCohost != nil {
		target = earliestCohost
	}
	if target == nil {
		extras := append(append([]string{}, ids...), leaveUserID)
		return m.endLocked(ctx, room, extras...)
	}
	return m.transferAdminLocked(ctx, room, room.AdminUserID, target.UserID)
}

func (m *ConferenceManager) EndConference(roomID, userID string) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		if _, err := m.requireModerator(lockCtx, roomID, userID); err != nil {
			return err
		}
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return m.endLocked(lockCtx, room)
	})
}

// AdminForceEndConference ends a conference without host permission checks.
// Missing rooms are treated as already ended (idempotent).
func (m *ConferenceManager) AdminForceEndConference(roomID string) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return m.endLocked(lockCtx, room)
	})
}

func (m *ConferenceManager) endLocked(ctx context.Context, room *ConferenceRoom, extraNotifyIDs ...string) error {
	if room.EgressID != "" {
		_ = m.lk.StopEgress(ctx, room.EgressID)
	}
	if room.IngressID != "" {
		_ = m.lk.DeleteIngress(ctx, room.IngressID)
	}
	// Collect recipients while membership still exists, then notify before tearing
	// down media. Deleting LiveKit first makes peers reconnect instead of exiting.
	// Use a detached ctx so lock/request cancellation cannot abort the ended fan-out.
	ids, _ := m.store.AllMemberIDs(ctx, room.RoomID)
	notifyIDs := mergeUniqueUserIDs(ids, extraNotifyIDs...)
	endedAt := time.Now().UnixMilli()
	notifyCtx := context.Background()
	m.broadcastEnded(notifyCtx, room.RoomID, notifyIDs)
	m.broadcastAnnounceEnded(notifyCtx, room, endedAt, extraNotifyIDs...)
	m.notifyConferenceClosed(notifyCtx, room, extraNotifyIDs...)
	occID := room.ScheduleOccurrenceID
	roomID := room.RoomID
	if m.waiting != nil {
		m.waiting.Clear(notifyCtx, roomID)
	}
	_ = m.store.DeleteRoomAll(ctx, room.RoomID)
	_ = m.lk.DeleteRoom(ctx, room.RoomID)
	if m.onRoomEnded != nil {
		m.onRoomEnded(roomID, occID)
	}
	return nil
}

// JoinConferenceResult join 结构化结果（含等候室旁路）。
type JoinConferenceResult struct {
	Status       string // ongoing | waiting
	Token        string
	Room         *ConferenceRoom
	Member       *ConferenceMember
	WaitingCount int64
}

// JoinConferenceWithOpts 支持密码与等候室的入会。
// passwordVerified：上层（如 ScheduleService）已校验通过时跳过二次 bcrypt。
func (m *ConferenceManager) JoinConferenceWithOpts(roomID, userID, password string, passwordVerified bool) (*JoinConferenceResult, error) {
	ctx := context.Background()
	room, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("conference %s not found", roomID)
	}
	isHost := userID == room.AdminUserID || userID == room.CreatorUserID
	// Existing members may rejoin without password.
	if _, exists, _ := m.store.GetMember(ctx, roomID, userID); exists {
		tok, room2, mem, err := m.JoinConference(roomID, userID)
		if err != nil {
			return nil, err
		}
		return &JoinConferenceResult{Status: "ongoing", Token: tok, Room: room2, Member: mem}, nil
	}
	// Host is exempt; skip if caller already verified.
	if room.PasswordHash != "" && !isHost && !passwordVerified {
		if password == "" {
			return nil, fmt.Errorf("password_required")
		}
		if m.pwdLimiter != nil {
			allowed, _ := m.pwdLimiter.CheckAllowed(ctx, roomID, userID)
			if !allowed {
				return nil, fmt.Errorf("password_rate_limited")
			}
		}
		if !VerifyMeetingPassword(room.PasswordHash, password) {
			if m.pwdLimiter != nil {
				m.pwdLimiter.RecordFailure(ctx, roomID, userID)
			}
			return nil, fmt.Errorf("wrong_password")
		}
		if m.pwdLimiter != nil {
			m.pwdLimiter.ClearFailures(ctx, roomID, userID)
		}
	}

	if room.WaitingRoomEnabled && !isHost {
		admitted := false
		if m.waiting != nil {
			admitted, _ = m.waiting.IsAdmitted(ctx, roomID, userID)
		}
		if !admitted {
			if m.waiting == nil {
				return nil, fmt.Errorf("waiting room unavailable")
			}
			nickname, face := "", ""
			if m.im != nil {
				if tok, e := m.im.ImAdminTokenWithDefaultAdmin(ctx); e == nil {
					if u, e2 := m.im.GetUserInfo(mctx.WithApiToken(ctx, tok), userID); e2 == nil && u != nil {
						nickname, face = u.Nickname, u.FaceURL
					}
				}
			}
			count, already, err := m.waiting.Enqueue(ctx, roomID, userID, nickname, face)
			if err != nil {
				return nil, err
			}
			if !already {
				m.waiting.NotifyHostWaiting(ctx, room.AdminUserID, roomID, "waiting_updated", []string{userID})
				return &JoinConferenceResult{Status: "waiting", WaitingCount: count, Room: room}, nil
			}
		}
	}

	tok, room2, mem, err := m.JoinConference(roomID, userID)
	if err != nil {
		return nil, err
	}
	if m.waiting != nil {
		m.waiting.ConsumeAdmitted(ctx, roomID, userID)
	}
	return &JoinConferenceResult{Status: "ongoing", Token: tok, Room: room2, Member: mem}, nil
}

func mergeUniqueUserIDs(base []string, extras ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(extras))
	out := make([]string, 0, len(base)+len(extras))
	for _, id := range base {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range extras {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (m *ConferenceManager) GetConference(roomID string) (*ConferenceRoom, bool) {
	room, ok, err := m.store.GetRoom(context.Background(), roomID)
	if err != nil || !ok {
		return nil, false
	}
	return room, true
}

func (m *ConferenceManager) GetMember(roomID, userID string) (*ConferenceMember, bool) {
	mem, ok, err := m.store.GetMember(context.Background(), roomID, userID)
	if err != nil || !ok {
		return nil, false
	}
	return mem, true
}

func (m *ConferenceManager) IsMember(roomID, userID string) bool {
	_, ok := m.GetMember(roomID, userID)
	return ok
}

// MemberDisplayName returns the in-room nickname for red-packet labels.
// Empty when unknown or when Nickname is just the userID (not a real display name).
func (m *ConferenceManager) MemberDisplayName(roomID, userID string) string {
	mem, ok := m.GetMember(roomID, userID)
	if !ok || mem == nil {
		return ""
	}
	nick := strings.TrimSpace(mem.Nickname)
	if nick == "" || nick == userID {
		return ""
	}
	return nick
}

func (m *ConferenceManager) requireAdmin(ctx context.Context, roomID, userID string) (*ConferenceRoom, error) {
	room, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("conference not found")
	}
	if room.AdminUserID != userID {
		return nil, fmt.Errorf("only admin allowed")
	}
	return room, nil
}

// requireModerator allows the room host or a co-host.
func (m *ConferenceManager) requireModerator(ctx context.Context, roomID, userID string) (*ConferenceRoom, error) {
	room, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("conference not found")
	}
	if room.AdminUserID == userID {
		return room, nil
	}
	mem, ok, err := m.store.GetMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !ok || mem == nil || mem.Role != ConferenceRoleCohost {
		return nil, fmt.Errorf("only moderator allowed")
	}
	return room, nil
}

func (m *ConferenceManager) Invite(roomID, adminID string, userIDs []string) error {
	ctx := context.Background()
	var roomSnap *ConferenceRoom
	var newInvitees []string
	err := m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, adminID)
		if err != nil {
			return err
		}
		seen := map[string]struct{}{}
		for _, id := range room.InviteeUserIDs {
			seen[id] = struct{}{}
		}
		for _, id := range userIDs {
			if id == "" || id == adminID {
				continue
			}
			if _, ok := seen[id]; !ok {
				room.InviteeUserIDs = append(room.InviteeUserIDs, id)
				seen[id] = struct{}{}
				newInvitees = append(newInvitees, id)
			}
			_ = m.store.ClearDeny(lockCtx, roomID, id)
		}
		if err := m.bumpAndSync(lockCtx, room, "invited", ""); err != nil {
			return err
		}
		roomSnap = room
		return nil
	})
	if err == nil && roomSnap != nil {
		m.sendConferenceInvites(ctx, roomSnap, adminID, newInvitees)
	}
	return err
}

func (m *ConferenceManager) TransferAdmin(roomID, adminID, targetUserID string) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireAdmin(lockCtx, roomID, adminID)
		if err != nil {
			return err
		}
		return m.transferAdminLocked(lockCtx, room, adminID, targetUserID)
	})
}

func (m *ConferenceManager) transferAdminLocked(ctx context.Context, room *ConferenceRoom, oldAdmin, newAdmin string) error {
	newMem, ok, err := m.store.GetMember(ctx, room.RoomID, newAdmin)
	if err != nil || !ok {
		return fmt.Errorf("target not in conference")
	}
	oldMem, ok, _ := m.store.GetMember(ctx, room.RoomID, oldAdmin)
	if ok {
		oldMem.Role = ConferenceRoleUser
		_ = m.store.SaveMember(ctx, room.RoomID, oldMem)
		m.syncParticipantPublishPerm(ctx, room, oldMem, memberCanPublishData(room, oldMem))
	}
	newMem.Role = ConferenceRoleAdmin
	if !newMem.OnStage {
		newMem.OnStage = true
	}
	newMem.SpeakPending = false
	if err := m.store.SaveMember(ctx, room.RoomID, newMem); err != nil {
		return err
	}
	room.AdminUserID = newAdmin
	m.syncParticipantPublishPerm(ctx, room, newMem, true)
	payload := BuildConferenceSync(room, "admin_transferred", newAdmin)
	m.attachStageState(ctx, room, payload, newMem)
	return m.bumpAndSyncPayload(ctx, room, payload)
}

// Kick disconnects target and denies rejoin for this room.
func (m *ConferenceManager) Kick(roomID, hostID, targetUserID string) error {
	return m.eject(roomID, hostID, targetUserID, true)
}

// Remove disconnects target without denying rejoin.
func (m *ConferenceManager) Remove(roomID, hostID, targetUserID string) error {
	return m.eject(roomID, hostID, targetUserID, false)
}

func (m *ConferenceManager) eject(roomID, hostID, targetUserID string, denyRejoin bool) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		if targetUserID == hostID {
			return fmt.Errorf("cannot kick self")
		}
		if targetUserID == room.AdminUserID {
			return fmt.Errorf("cannot kick admin")
		}
		_ = m.lk.RemoveParticipant(lockCtx, roomID, targetUserID)
		if denyRejoin {
			_ = m.store.AddDeny(lockCtx, roomID, targetUserID)
		}
		m.clearMemberStageOnLeave(lockCtx, room, targetUserID)
		if err := m.store.RemoveMember(lockCtx, roomID, targetUserID); err != nil {
			return err
		}
		n, _ := m.store.MemberCount(lockCtx, roomID)
		room.RefreshParticipantCount(int(n))
		_ = m.store.SaveRoom(lockCtx, room)
		event := "removed"
		if denyRejoin {
			event = "kicked"
		}
		payload := BuildConferenceSync(room, event, targetUserID)
		m.attachStageState(lockCtx, room, payload, nil)
		return m.bumpAndSyncPayload(lockCtx, room, payload, targetUserID)
	})
}

func (m *ConferenceManager) SetMedia(roomID, hostID, targetUserID string, audio *bool, video *bool) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		mem, ok, err := m.store.GetMember(lockCtx, roomID, targetUserID)
		if err != nil || !ok {
			return fmt.Errorf("member not found")
		}
		if audio != nil {
			mem.AudioForcedOff = !*audio
			if mem.AudioForcedOff {
				m.forceMuteMediaKind(lockCtx, roomID, targetUserID, false)
			}
		}
		if video != nil {
			mem.VideoForcedOff = !*video
			if mem.VideoForcedOff {
				m.forceMuteMediaKind(lockCtx, roomID, targetUserID, true)
			}
		}
		if err := m.store.SaveMember(lockCtx, roomID, mem); err != nil {
			return err
		}
		m.syncParticipantPublishPerm(lockCtx, room, mem, memberCanPublishData(room, mem))
		payload := BuildConferenceSync(room, "media", targetUserID)
		payload.TargetAudioForcedOff = mem.AudioForcedOff
		payload.TargetVideoForcedOff = mem.VideoForcedOff
		return m.bumpAndSyncPayload(lockCtx, room, payload)
	})
}

func (m *ConferenceManager) SetMediaAll(roomID, hostID string, audio *bool, video *bool) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		if audio != nil {
			room.GlobalAudioMuted = !*audio
		}
		if video != nil {
			room.GlobalVideoOff = !*video
		}
		ids, _ := m.store.AllMemberIDs(lockCtx, roomID)
		for _, id := range ids {
			mem, ok, _ := m.store.GetMember(lockCtx, roomID, id)
			if !ok {
				continue
			}
			// The room-wide flags are evaluated on their own in mediaAllowFlags /
			// canOpenCamera, so they must not be copied into per-member flags:
			// media_all carries no per-member state for clients to undo, and
			// "unmute all" would silently clear a host's individual restriction.
			// Host / co-host are not force-muted (they may re-open mic while mute-all
			// stays on), but still get publish permissions synced.
			if !memberIsModerator(room, mem) {
				if audio != nil && room.GlobalAudioMuted {
					m.forceMuteMediaKind(lockCtx, roomID, id, false)
				}
				if video != nil && room.GlobalVideoOff {
					m.forceMuteMediaKind(lockCtx, roomID, id, true)
				}
			}
			m.syncParticipantPublishPerm(lockCtx, room, mem, memberCanPublishData(room, mem))
		}
		return m.bumpAndSync(lockCtx, room, "media_all", "")
	})
}

func (m *ConferenceManager) SetLocked(roomID, hostID string, locked bool) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		room.Locked = locked
		return m.bumpAndSync(lockCtx, room, "lock", "")
	})
}

func (m *ConferenceManager) SetChatMuted(roomID, hostID string, muted bool) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		room.ChatMuted = muted
		ids, _ := m.store.AllMemberIDs(lockCtx, roomID)
		for _, id := range ids {
			mem, ok, _ := m.store.GetMember(lockCtx, roomID, id)
			if !ok {
				continue
			}
			m.syncParticipantPublishPerm(lockCtx, room, mem, memberCanPublishData(room, mem))
		}
		return m.bumpAndSync(lockCtx, room, "chat_mute", "")
	})
}

// SetMemberChatMuted 房主/联席对单个成员开启/解除文字禁言（不影响音视频）。
func (m *ConferenceManager) SetMemberChatMuted(roomID, hostID, targetUserID string, muted bool) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		if targetUserID == "" || targetUserID == room.AdminUserID {
			return fmt.Errorf("cannot mute admin chat")
		}
		mem, ok, err := m.store.GetMember(lockCtx, roomID, targetUserID)
		if err != nil || !ok {
			return fmt.Errorf("member not found")
		}
		mem.ChatForcedOff = muted
		if err := m.store.SaveMember(lockCtx, roomID, mem); err != nil {
			return err
		}
		m.syncParticipantPublishPerm(lockCtx, room, mem, memberCanPublishData(room, mem))
		payload := BuildConferenceSync(room, "member_chat_mute", targetUserID)
		payload.TargetChatForcedOff = mem.ChatForcedOff
		return m.bumpAndSyncPayload(lockCtx, room, payload)
	})
}

// RevokeChat 广播弹幕撤回：本人可撤自己的，房主/联席可撤任意人的（不依赖 LiveKit DataChannel）。
func (m *ConferenceManager) RevokeChat(roomID, operatorID, targetSenderID string, seq uint64) error {
	ctx := context.Background()
	if targetSenderID == "" || seq == 0 {
		return fmt.Errorf("targetUserID and seq required")
	}
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok || room == nil {
			return fmt.Errorf("conference not found")
		}
		op, ok, err := m.store.GetMember(lockCtx, roomID, operatorID)
		if err != nil || !ok || op == nil {
			return fmt.Errorf("not in conference")
		}
		if !memberIsModerator(room, op) && operatorID != targetSenderID {
			return fmt.Errorf("no permission to revoke")
		}
		payload := BuildConferenceSync(room, "chat_revoke", targetSenderID)
		payload.RevokeTargetSeq = seq
		return m.bumpAndSyncPayload(lockCtx, room, payload)
	})
}

func (m *ConferenceManager) Pin(roomID, hostID, targetUserID string) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		room.PinnedUserID = targetUserID
		return m.bumpAndSync(lockCtx, room, "pin", targetUserID)
	})
}

func (m *ConferenceManager) StartEgress(roomID, hostID, filepath string) (string, error) {
	ctx := context.Background()
	var egressID string
	err := m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		if filepath == "" {
			filepath = fmt.Sprintf("conference/%s/%d.mp4", roomID, time.Now().Unix())
		}
		id, err := m.lk.StartRoomCompositeEgress(lockCtx, roomID, filepath)
		if err != nil {
			return err
		}
		room.EgressID = id
		egressID = id
		return m.bumpAndSync(lockCtx, room, "egress_started", "")
	})
	return egressID, err
}

func (m *ConferenceManager) StopEgress(roomID, hostID string) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		if room.EgressID == "" {
			return nil
		}
		_ = m.lk.StopEgress(lockCtx, room.EgressID)
		room.EgressID = ""
		return m.bumpAndSync(lockCtx, room, "egress_stopped", "")
	})
}

func (m *ConferenceManager) CreateIngress(roomID, hostID, name, inputType string) (ingressID, url, streamKey string, err error) {
	ctx := context.Background()
	err = m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, e := m.requireModerator(lockCtx, roomID, hostID)
		if e != nil {
			return e
		}
		if name == "" {
			name = "ingress"
		}
		var info *livekit.IngressInfo
		switch normalizeIngressInputType(inputType) {
		case "whip":
			info, e = m.lk.CreateWHIPIngress(lockCtx, roomID, name)
		default:
			info, e = m.lk.CreateRTMPIngress(lockCtx, roomID, name)
		}
		if e != nil {
			return e
		}
		room.IngressID = info.IngressId
		room.IngressURL = info.Url
		room.IngressStreamKey = info.StreamKey
		ingressID, url, streamKey = info.IngressId, info.Url, info.StreamKey
		return m.bumpAndSync(lockCtx, room, "ingress_created", "")
	})
	return
}

func normalizeIngressInputType(inputType string) string {
	switch inputType {
	case "whip", "WHIP", "Whip":
		return "whip"
	default:
		return "rtmp"
	}
}

func (m *ConferenceManager) DeleteIngress(roomID, hostID string) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		if room.IngressID != "" {
			_ = m.lk.DeleteIngress(lockCtx, room.IngressID)
		}
		room.IngressID = ""
		room.IngressURL = ""
		room.IngressStreamKey = ""
		return m.bumpAndSync(lockCtx, room, "ingress_deleted", "")
	})
}

// SetCohost promotes or demotes a member as co-host (moderator only; cannot target room host).
func (m *ConferenceManager) SetCohost(roomID, operatorID, targetUserID string, cohost bool) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, operatorID)
		if err != nil {
			return err
		}
		if targetUserID == "" || targetUserID == room.AdminUserID {
			return fmt.Errorf("cannot change admin cohost status")
		}
		mem, ok, err := m.store.GetMember(lockCtx, roomID, targetUserID)
		if err != nil || !ok {
			return fmt.Errorf("target not in conference")
		}
		event := "cohost_unset"
		if cohost {
			mem.Role = ConferenceRoleCohost
			event = "cohost_set"
		} else {
			mem.Role = ConferenceRoleUser
		}
		if err := m.store.SaveMember(lockCtx, roomID, mem); err != nil {
			return err
		}
		m.syncParticipantPublishPerm(lockCtx, room, mem, memberCanPublishData(room, mem))
		payload := BuildConferenceSync(room, event, targetUserID)
		m.attachStageState(lockCtx, room, payload, mem)
		return m.bumpAndSyncPayload(lockCtx, room, payload)
	})
}

func (m *ConferenceManager) MarkInLiveKit(roomID, userID string) {
	ctx := context.Background()
	_ = m.store.CancelPendingLeave(ctx, roomID, userID)
	mem, ok, err := m.store.GetMember(ctx, roomID, userID)
	if err != nil || !ok {
		return
	}
	mem.InLiveKit = true
	_ = m.store.SaveMember(ctx, roomID, mem)
	room, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil || !ok || room == nil {
		return
	}
	// Push role/hand/mute onto LiveKit metadata for late-join peers.
	m.syncParticipantPublishPerm(ctx, room, mem, memberCanPublishData(room, mem))
}

func (m *ConferenceManager) DeleteRoomFinished(roomID string) {
	ctx := context.Background()
	if err := m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return m.endLocked(lockCtx, room)
	}); err != nil {
		log.ZWarn(ctx, "conference room_finished end failed", err, "roomID", roomID)
	}
}

func (m *ConferenceManager) cleanupLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.runCleanup(context.Background())
		}
	}
}

func (m *ConferenceManager) runCleanup(ctx context.Context) {
	ids, err := m.store.ListActiveRoomIDs(ctx)
	if err != nil {
		return
	}
	now := time.Now().Unix()
	for _, roomID := range ids {
		room, ok, err := m.store.GetRoom(ctx, roomID)
		if err != nil || !ok {
			continue
		}
		if now-room.CreateTime <= 8*3600 {
			continue
		}
		if err := m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
			latest, ok, err := m.store.GetRoom(lockCtx, roomID)
			if err != nil || !ok {
				return err
			}
			if now-latest.CreateTime <= 8*3600 {
				return nil
			}
			return m.endLocked(lockCtx, latest)
		}); err != nil {
			log.ZWarn(ctx, "conference cleanup end failed", err, "roomID", roomID)
		}
	}
}

// ListActiveConferences returns paginated ongoing conferences sorted by createTime desc.
func (m *ConferenceManager) ListActiveConferences(ctx context.Context, pageNumber, showNumber int32) (int64, []*ActiveConferenceSummary, error) {
	if pageNumber < 1 {
		pageNumber = 1
	}
	if showNumber < 1 {
		showNumber = 20
	}
	ids, err := m.store.ListActiveRoomIDs(ctx)
	if err != nil {
		return 0, nil, err
	}
	list := make([]*ActiveConferenceSummary, 0, len(ids))
	for _, roomID := range ids {
		room, ok, err := m.store.GetRoom(ctx, roomID)
		if err != nil || !ok || room == nil {
			continue
		}
		realN, _ := m.store.MemberCount(ctx, roomID)
		r := int(realN)
		boost := room.DisplayBoost
		if boost < 0 {
			boost = 0
		}
		list = append(list, &ActiveConferenceSummary{
			RoomID:               room.RoomID,
			Title:                room.Title,
			AdminUserID:          room.AdminUserID,
			CreatorUserID:        room.CreatorUserID,
			CallType:             room.CallType,
			GroupID:              room.GroupID,
			CreateTime:           room.CreateTime,
			RealParticipantCount: r,
			DisplayBoost:         boost,
			ParticipantCount:     r + boost,
			Locked:               room.Locked,
			HasPassword:          room.PasswordHash != "",
			Rev:                  room.Rev,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreateTime > list[j].CreateTime
	})
	total := int64(len(list))
	start := int((pageNumber - 1) * showNumber)
	if start >= len(list) {
		return total, []*ActiveConferenceSummary{}, nil
	}
	end := start + int(showNumber)
	if end > len(list) {
		end = len(list)
	}
	return total, list[start:end], nil
}

// AdjustDisplayBoost changes the admin virtual participant boost and fans out conference_sync.
// addCount may be negative to reduce boost (floored at 0). Redis writes happen under the room
// lock; IM fan-out runs after the lock is released.
// ctx should carry operationID (e.g. gin request context) so IM notifications are accepted.
func (m *ConferenceManager) AdjustDisplayBoost(ctx context.Context, roomID string, addCount int) (*ActiveConferenceSummary, error) {
	if addCount == 0 {
		return nil, fmt.Errorf("addCount must be non-zero")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var out *ActiveConferenceSummary
	var payload *ConferenceSyncPayload
	var memberIDs []string
	var boostBefore int
	err := m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok || room == nil {
			return fmt.Errorf("conference room not found")
		}
		if room.DisplayBoost < 0 {
			room.DisplayBoost = 0
		}
		boostBefore = room.DisplayBoost
		room.DisplayBoost += addCount
		if room.DisplayBoost < 0 {
			room.DisplayBoost = 0
		}
		n, _ := m.store.MemberCount(lockCtx, roomID)
		room.RefreshParticipantCount(int(n))
		payload = BuildConferenceSync(room, "count_adjusted", "")
		if err := m.store.BumpRev(lockCtx, room); err != nil {
			return err
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
		m.attachStageState(lockCtx, room, payload, nil)
		memberIDs = append([]string(nil), m.memberIDsForSync(lockCtx, room.RoomID)...)
		out = &ActiveConferenceSummary{
			RoomID:               room.RoomID,
			Title:                room.Title,
			AdminUserID:          room.AdminUserID,
			CreatorUserID:        room.CreatorUserID,
			CallType:             room.CallType,
			GroupID:              room.GroupID,
			CreateTime:           room.CreateTime,
			RealParticipantCount: int(n),
			DisplayBoost:         room.DisplayBoost,
			ParticipantCount:     room.ParticipantCount,
			Locked:               room.Locked,
			Rev:                  room.Rev,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	attempted, failed := m.broadcastSync(ctx, memberIDs, payload)
	notified := attempted - failed
	if notified < 0 {
		notified = 0
	}
	if out != nil {
		out.NotifiedCount = notified
		out.NotifyFailed = failed
	}
	if attempted == 0 {
		log.ZWarn(ctx, "conference count_adjusted notify skipped: no recipients", nil,
			"roomID", roomID, "addCount", addCount, "boostBefore", boostBefore,
			"boostAfter", out.DisplayBoost, "realCount", out.RealParticipantCount, "rev", out.Rev)
	} else if failed == attempted {
		log.ZError(ctx, "conference count_adjusted notify all failed", nil,
			"roomID", roomID, "addCount", addCount, "boostBefore", boostBefore,
			"boostAfter", out.DisplayBoost, "recipients", attempted, "rev", out.Rev)
	} else if failed > 0 {
		log.ZWarn(ctx, "conference count_adjusted notify partial failure", nil,
			"roomID", roomID, "addCount", addCount, "boostBefore", boostBefore,
			"boostAfter", out.DisplayBoost, "notified", notified, "failed", failed, "rev", out.Rev)
	} else {
		log.ZInfo(ctx, "conference count_adjusted notified",
			"roomID", roomID, "addCount", addCount, "boostBefore", boostBefore,
			"boostAfter", out.DisplayBoost, "realCount", out.RealParticipantCount,
			"participantCount", out.ParticipantCount, "notified", notified, "rev", out.Rev)
	}
	return out, nil
}
