package rtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"chat/pkg/common/imapi"
	"gitee.com/matkurban/openimsdk-tools/log"
	"github.com/google/uuid"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/redis/go-redis/v9"
)

const (
	MaxParticipants = 9
	// 无人进入 LiveKit 的废弃房间超时；进行中通话不能用这个值删。
	staleRoomTimeoutSec = 600
	// 进行中通话的硬上限兜底（webhook/客户端漏收尾时防止房间永久悬挂）。
	hardRoomTimeoutSec  = 8 * 3600
	cleanupLoopInterval = time.Minute
	neverJoinedGraceSec = 90
)

// MeetingManager 会议管理器（Redis 共享状态 + LiveKit token）
type MeetingManager struct {
	store       *RedisMeetingStore
	key         string
	secret      string
	url         string
	im          imapi.CallerInterface
	adminUserID string
	eventCh     chan *livekit.WebhookEvent
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewMeetingManager 创建会议管理器；Redis 为必需依赖。
func NewMeetingManager(
	rdb redis.UniversalClient,
	key, secret, url string,
	im imapi.CallerInterface,
	adminUserID string,
) *MeetingManager {
	m := &MeetingManager{
		store:       NewRedisMeetingStore(rdb),
		key:         key,
		secret:      secret,
		url:         url,
		im:          im,
		adminUserID: adminUserID,
		eventCh:     make(chan *livekit.WebhookEvent, 256),
		stopCh:      make(chan struct{}),
	}
	m.wg.Add(1)
	go m.cleanupLoop()
	m.wg.Add(1)
	go m.webhookWorker()
	return m
}

// Stop shuts down background workers.
func (m *MeetingManager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// GetLiveKitURL 获取 LiveKit 服务器地址
func (m *MeetingManager) GetLiveKitURL() string {
	return m.url
}

func (m *MeetingManager) LiveKitKey() string    { return m.key }
func (m *MeetingManager) LiveKitSecret() string { return m.secret }

func (m *MeetingManager) generateToken(roomID, userID string) (string, error) {
	at := auth.NewAccessToken(m.key, m.secret)
	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     roomID,
	}
	at.AddGrant(grant).
		SetIdentity(userID).
		SetValidFor(24 * time.Hour)
	return at.ToJWT()
}

// CreateMeeting 创建会议房间
func (m *MeetingManager) CreateMeeting(creatorUserID, callType string, inviteeUserIDs []string) (*MeetingRoom, string, error) {
	ctx := context.Background()
	if err := m.store.clearUserStaleRoom(ctx, creatorUserID, ""); err != nil {
		return nil, "", err
	}

	roomID := uuid.New().String()
	now := time.Now().Unix()
	room := &MeetingRoom{
		RoomID:         roomID,
		CreatorUserID:  creatorUserID,
		CallType:       callType,
		InviteeUserIDs: inviteeUserIDs,
		Participants: map[string]*MeetingParticipant{
			creatorUserID: {
				UserID:   creatorUserID,
				JoinTime: now,
			},
		},
		CreateTime: now,
	}

	token, err := m.generateToken(roomID, creatorUserID)
	if err != nil {
		return nil, "", fmt.Errorf("generate token error: %w", err)
	}

	if err := m.store.saveRoom(ctx, room); err != nil {
		return nil, "", err
	}
	if err := m.store.setUserRoom(ctx, creatorUserID, roomID); err != nil {
		_ = m.store.deleteRoom(ctx, room)
		return nil, "", err
	}

	return room, token, nil
}

// JoinMeeting 加入会议房间
func (m *MeetingManager) JoinMeeting(roomID, userID string) (string, *MeetingRoom, error) {
	ctx := context.Background()
	if err := m.store.clearUserStaleRoom(ctx, userID, roomID); err != nil {
		return "", nil, err
	}

	room, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, fmt.Errorf("room %s not found", roomID)
	}

	if len(room.Participants) >= MaxParticipants {
		return "", nil, fmt.Errorf("room %s is full (max %d)", roomID, MaxParticipants)
	}

	room.Participants[userID] = &MeetingParticipant{
		UserID:   userID,
		JoinTime: time.Now().Unix(),
	}

	token, err := m.generateToken(roomID, userID)
	if err != nil {
		delete(room.Participants, userID)
		return "", nil, fmt.Errorf("generate token error: %w", err)
	}

	if err := m.store.saveRoom(ctx, room); err != nil {
		delete(room.Participants, userID)
		return "", nil, err
	}
	if err := m.store.setUserRoom(ctx, userID, roomID); err != nil {
		delete(room.Participants, userID)
		_ = m.store.saveRoom(ctx, room)
		return "", nil, err
	}

	return token, room, nil
}

// LeaveMeeting 离开会议房间。
//
// 幂等：房间已不存在或调用者本就不是参与者时，只清掉自己的忙线标记并返回成功，
// 这样客户端补发的归还请求不会失败，孤儿 meeting:user key 也能顺手清掉。
//
// 1v1 房间只要有真实参与者离开就整房结束，并给对端补发 hangup 信令。否则非创建者
// 离开时房间与创建者的 meeting:user key 会一起悬挂，导致创建者之后一直被判忙线、
// 收不到任何来电（唯一的解除途径是它自己再拨一次电话）。
func (m *MeetingManager) LeaveMeeting(roomID, userID string) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok {
			return m.store.rdb.Del(lockCtx, userKey(userID)).Err()
		}
		if _, isParticipant := room.Participants[userID]; !isParticipant {
			return m.store.rdb.Del(lockCtx, userKey(userID)).Err()
		}

		roomBefore := new(*room)
		if err := m.leaveMeetingLocked(lockCtx, room, userID); err != nil {
			return err
		}

		if len(roomBefore.InviteeUserIDs) != 1 {
			return nil
		}
		if _, err := m.endMeetingLocked(lockCtx, roomBefore.RoomID, roomBefore.CreatorUserID); err != nil {
			log.ZWarn(lockCtx, "leave meeting end 1v1 room failed", err, "roomID", roomID)
		}
		// 客户端的 hangup 信令是 onlineOnly 的，丢一次对端就会永远卡在通话中；
		// 这里补一发，重复的 hangup 在对端会因无匹配 session 被忽略。
		if peerID := abnormalLeavePeerID(roomBefore, userID); peerID != "" {
			m.sendHangupSignal(lockCtx, roomBefore, userID, peerID)
		}
		return nil
	})
}

// EndMeeting 结束会议，返回所有参与者 ID。房间已被回收时按成功处理（幂等）。
func (m *MeetingManager) EndMeeting(roomID, userID string) ([]string, error) {
	ctx := context.Background()
	room, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !ok {
		// 房间可能已被 webhook 或清理协程回收；客户端补发的归还不应报错，
		// 但仍要保证调用者的忙线标记被清掉。
		_ = m.store.rdb.Del(ctx, userKey(userID))
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

// GetMeeting 获取会议信息
func (m *MeetingManager) GetMeeting(roomID string) (*MeetingRoom, bool) {
	room, ok, err := m.store.GetRoom(context.Background(), roomID)
	if err != nil || !ok {
		return nil, false
	}
	return room, true
}

// GetUserActiveMeeting 获取用户当前所在的房间
func (m *MeetingManager) GetUserActiveMeeting(userID string) (*MeetingRoom, bool) {
	ctx := context.Background()
	room, ok := m.resolveUserRoom(ctx, userID)
	return room, ok
}

// GetBusyUsers 返回正在通话中的用户
func (m *MeetingManager) GetBusyUsers(userIDs []string) []string {
	ctx := context.Background()
	busy := make([]string, 0)
	for _, uid := range userIDs {
		if _, ok := m.resolveUserRoom(ctx, uid); ok {
			busy = append(busy, uid)
		}
	}
	return busy
}

// resolveUserRoom 解析用户的忙线标记，并自愈孤儿 key。
//
// meeting:user 与 meeting:room 是两条独立 TTL 的 key，房间先过期或被回收时忙线标记
// 会残留。残留期间该用户会被 /meeting/create 判为 busyUsers，导致 1v1 邀请直接失败、
// 对方连来电都收不到，所以这里发现房间已不存在就立刻删掉标记。
func (m *MeetingManager) resolveUserRoom(ctx context.Context, userID string) (*MeetingRoom, bool) {
	roomID, ok, err := m.store.getUserRoomID(ctx, userID)
	if err != nil || !ok {
		return nil, false
	}
	room, found, err := m.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, false
	}
	if !found {
		log.ZInfo(ctx, "meeting clear orphan user busy key", "userID", userID, "roomID", roomID)
		_ = m.store.rdb.Del(ctx, userKey(userID))
		return nil, false
	}
	return room, true
}

// IsParticipantActive 用户是否仍在 meeting 中（用于 webhook 区分正常挂断 vs 异常离开）
func (m *MeetingManager) IsParticipantActive(roomID, userID string) bool {
	active, err := m.store.IsParticipantActive(context.Background(), roomID, userID)
	return err == nil && active
}

// MarkParticipantInLiveKit 标记用户已进入 LiveKit 房间
func (m *MeetingManager) MarkParticipantInLiveKit(roomID, userID string) {
	if err := m.store.MarkParticipantInLiveKit(context.Background(), roomID, userID); err != nil {
		log.ZWarn(context.Background(), "MarkParticipantInLiveKit failed", err, "roomID", roomID, "userID", userID)
	}
}

// DeleteRoomFinished 删除已结束的 LiveKit 房间（幂等）
func (m *MeetingManager) DeleteRoomFinished(roomID string) {
	ctx := context.Background()
	room, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil || !ok {
		return
	}
	_ = m.store.deleteRoom(ctx, room)
}

func (m *MeetingManager) cleanupLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(cleanupLoopInterval)
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

func (m *MeetingManager) runCleanup(ctx context.Context) {
	roomIDs, err := m.store.ListActiveRoomIDs(ctx)
	if err != nil {
		log.ZWarn(ctx, "meeting cleanup list active rooms failed", err)
		return
	}
	now := time.Now().Unix()
	for _, roomID := range roomIDs {
		room, ok, err := m.store.GetRoom(ctx, roomID)
		if err != nil {
			continue
		}
		if !ok {
			// active 集合里的孤儿 roomID：房间 key 已过期或被回收。
			_ = m.store.rdb.SRem(ctx, meetingActiveKey, roomID)
			continue
		}

		age := now - room.CreateTime
		// 只回收「从未有人真正进入 LiveKit」的废弃房间。进行中的通话不能按 600s 删，
		// 否则超过该时长后 leave/end 全部 room not found，忙线标记也会一起失准。
		// 进行中通话交给 webhook / 客户端收尾，这里仅用更大的硬上限兜底防永久悬挂。
		if age > hardRoomTimeoutSec || (age > staleRoomTimeoutSec && !hasParticipantInLiveKit(room)) {
			_ = m.store.deleteRoom(ctx, room)
			continue
		}

		for uid, p := range room.Participants {
			if p.InLiveKit {
				continue
			}
			if now-p.JoinTime >= neverJoinedGraceSec {
				log.ZInfo(ctx, "meeting cleanup never joined livekit", "roomID", roomID, "userID", uid)
				_ = m.HandleAbnormalLeave(ctx, roomID, uid, "cleanup_never_joined")
			}
		}
	}
}

func hasParticipantInLiveKit(room *MeetingRoom) bool {
	for _, p := range room.Participants {
		if p.InLiveKit {
			return true
		}
	}
	return false
}
