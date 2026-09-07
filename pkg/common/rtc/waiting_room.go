package rtc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"gitee.com/matkurban/openimsdk-tools/log"
	"github.com/redis/go-redis/v9"
)

const (
	waitingRoomMax         = 200
	waitingAdmittedTTL     = 30 * time.Minute
	conferenceWaitingBizID = "conference_waiting"
)

func confWaitingKey(roomID string) string  { return "conference:waiting:" + roomID }
func confAdmittedKey(roomID string) string { return "conference:admitted:" + roomID }
func confWaitingMetaKey(roomID, userID string) string {
	return "conference:waiting:meta:" + roomID + ":" + userID
}

// WaitingUser 等候室成员摘要
type WaitingUser struct {
	UserID    string `json:"userID"`
	Nickname  string `json:"nickname,omitempty"`
	FaceURL   string `json:"faceURL,omitempty"`
	EnqueueAt int64  `json:"enqueueAt"`
}

// WaitingRoomStore Redis 独立等候室（SFU 旁路）
type WaitingRoomStore struct {
	rdb         redis.UniversalClient
	im          imapi.CallerInterface
	adminUserID string
}

func NewWaitingRoomStore(rdb redis.UniversalClient, im imapi.CallerInterface, adminUserID string) *WaitingRoomStore {
	return &WaitingRoomStore{rdb: rdb, im: im, adminUserID: adminUserID}
}

// Enqueue 将用户放入等候室；已在 admitted 则返回 false（可直接入会）。
func (s *WaitingRoomStore) Enqueue(ctx context.Context, roomID, userID, nickname, faceURL string) (waitingCount int64, alreadyAdmitted bool, err error) {
	if s == nil || s.rdb == nil {
		return 0, false, fmt.Errorf("waiting room store unavailable")
	}
	ok, err := s.rdb.SIsMember(ctx, confAdmittedKey(roomID), userID).Result()
	if err != nil {
		return 0, false, err
	}
	if ok {
		return 0, true, nil
	}
	// Already in waiting queue: return current count (do not hit waiting_full).
	if _, zerr := s.rdb.ZScore(ctx, confWaitingKey(roomID), userID).Result(); zerr == nil {
		n, _ := s.rdb.ZCard(ctx, confWaitingKey(roomID)).Result()
		return n, false, nil
	}
	n, err := s.rdb.ZCard(ctx, confWaitingKey(roomID)).Result()
	if err != nil {
		return 0, false, err
	}
	if n >= waitingRoomMax {
		return n, false, fmt.Errorf("waiting_full")
	}
	now := time.Now().UnixMilli()
	if err := s.rdb.ZAdd(ctx, confWaitingKey(roomID), redis.Z{Score: float64(now), Member: userID}).Err(); err != nil {
		return 0, false, err
	}
	meta, _ := json.Marshal(WaitingUser{UserID: userID, Nickname: nickname, FaceURL: faceURL, EnqueueAt: now})
	_ = s.rdb.Set(ctx, confWaitingMetaKey(roomID, userID), meta, 8*time.Hour).Err()
	n, _ = s.rdb.ZCard(ctx, confWaitingKey(roomID)).Result()
	return n, false, nil
}

// IsAdmitted 是否已准入。
func (s *WaitingRoomStore) IsAdmitted(ctx context.Context, roomID, userID string) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, nil
	}
	return s.rdb.SIsMember(ctx, confAdmittedKey(roomID), userID).Result()
}

// ConsumeAdmitted 入会成功后清除 admitted 标记。
func (s *WaitingRoomStore) ConsumeAdmitted(ctx context.Context, roomID, userID string) {
	if s == nil || s.rdb == nil {
		return
	}
	_ = s.rdb.SRem(ctx, confAdmittedKey(roomID), userID).Err()
	_ = s.rdb.ZRem(ctx, confWaitingKey(roomID), userID).Err()
	_ = s.rdb.Del(ctx, confWaitingMetaKey(roomID, userID)).Err()
}

// List 等候列表（按入队时间）。
func (s *WaitingRoomStore) List(ctx context.Context, roomID string) ([]WaitingUser, error) {
	if s == nil || s.rdb == nil {
		return nil, fmt.Errorf("waiting room store unavailable")
	}
	ids, err := s.rdb.ZRange(ctx, confWaitingKey(roomID), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	out := make([]WaitingUser, 0, len(ids))
	for _, id := range ids {
		raw, err := s.rdb.Get(ctx, confWaitingMetaKey(roomID, id)).Bytes()
		if err != nil {
			out = append(out, WaitingUser{UserID: id})
			continue
		}
		var u WaitingUser
		if json.Unmarshal(raw, &u) != nil {
			u = WaitingUser{UserID: id}
		}
		out = append(out, u)
	}
	return out, nil
}

// Admit 准入指定用户。
func (s *WaitingRoomStore) Admit(ctx context.Context, roomID string, userIDs []string) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("waiting room store unavailable")
	}
	if len(userIDs) == 0 {
		return nil
	}
	pipe := s.rdb.Pipeline()
	for _, uid := range userIDs {
		pipe.SAdd(ctx, confAdmittedKey(roomID), uid)
		pipe.ZRem(ctx, confWaitingKey(roomID), uid)
	}
	pipe.Expire(ctx, confAdmittedKey(roomID), waitingAdmittedTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// Reject 拒绝并写入 deny（复用 conference deny）。
func (s *WaitingRoomStore) Reject(ctx context.Context, store *RedisConferenceStore, roomID string, userIDs []string) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("waiting room store unavailable")
	}
	pipe := s.rdb.Pipeline()
	for _, uid := range userIDs {
		pipe.ZRem(ctx, confWaitingKey(roomID), uid)
		pipe.SRem(ctx, confAdmittedKey(roomID), uid)
		pipe.Del(ctx, confWaitingMetaKey(roomID, uid))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	if store != nil {
		for _, uid := range userIDs {
			_ = store.AddDeny(ctx, roomID, uid)
		}
	}
	return nil
}

// LeaveWaiting 成员主动离开等候室。
func (s *WaitingRoomStore) LeaveWaiting(ctx context.Context, roomID, userID string) error {
	if s == nil || s.rdb == nil {
		return nil
	}
	_ = s.rdb.ZRem(ctx, confWaitingKey(roomID), userID).Err()
	_ = s.rdb.Del(ctx, confWaitingMetaKey(roomID, userID)).Err()
	return nil
}

// Clear 会议结束清空等候室。
func (s *WaitingRoomStore) Clear(ctx context.Context, roomID string) {
	if s == nil || s.rdb == nil {
		return
	}
	ids, _ := s.rdb.ZRange(ctx, confWaitingKey(roomID), 0, -1).Result()
	pipe := s.rdb.Pipeline()
	pipe.Del(ctx, confWaitingKey(roomID))
	pipe.Del(ctx, confAdmittedKey(roomID))
	for _, id := range ids {
		pipe.Del(ctx, confWaitingMetaKey(roomID, id))
	}
	_, _ = pipe.Exec(ctx)
}

// ListAdmitted 返回已准入、待入会的用户 ID 列表。
func (s *WaitingRoomStore) ListAdmitted(ctx context.Context, roomID string) ([]string, error) {
	if s == nil || s.rdb == nil {
		return nil, fmt.Errorf("waiting room store unavailable")
	}
	return s.rdb.SMembers(ctx, confAdmittedKey(roomID)).Result()
}

// NotifyHostWaiting 通知主持人等候室变化。
func (s *WaitingRoomStore) NotifyHostWaiting(ctx context.Context, hostUserID, roomID, event string, targetUserIDs []string) {
	if s == nil || s.im == nil || hostUserID == "" {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"event":         event,
		"roomID":        roomID,
		"meetingID":     roomID,
		"targetUserIDs": targetUserIDs,
	})
	tok, err := s.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		log.ZWarn(ctx, "waiting notify token failed", err)
		return
	}
	apiCtx := mctx.WithApiToken(ctx, tok)
	if err := s.im.SendBusinessNotification(apiCtx, conferenceWaitingBizID, string(payload), s.adminUserID, hostUserID); err != nil {
		log.ZWarn(ctx, "waiting notify failed", err, "host", hostUserID)
	}
	for _, uid := range targetUserIDs {
		if uid == hostUserID {
			continue
		}
		_ = s.im.SendBusinessNotification(apiCtx, conferenceWaitingBizID, string(payload), s.adminUserID, uid)
	}
}
