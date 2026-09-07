package rtc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// releaseConferenceLockScript deletes the lock key only when the token matches.
var releaseConferenceLockScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0
`)

func randomConferenceLockToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

const (
	confRoomKeyPrefix         = "conference:room:"
	confMembersKeyPref        = "conference:members:"
	confMemberKeyPref         = "conference:member:"
	confUserKeyPrefix         = "conference:user:"
	confDenyKeyPrefix         = "conference:deny:"
	confLockKeyPrefix         = "conference:lock:"
	confWebhookKeyPrefix      = "conference:webhook:"
	confPendingLeaveClaimPref = "conference:pending_leave_claim:"
	confPendingLeavesKey      = "conference:pending_leaves"
	confActiveKey             = "conference:active"
	confRoomTTLDuration       = 8 * time.Hour
	confWebhookTTL            = 10 * time.Minute
	confPendingLeaveClaimTTL  = 15 * time.Second

	// conferenceDisconnectGrace is how long an abnormal LiveKit disconnect may
	// reconnect before the member is removed via LeaveConference.
	conferenceDisconnectGrace = 10 * time.Second
)

// pendingLeaveMember encodes roomID + userID for the pending-leaves ZSET.
func pendingLeaveMember(roomID, userID string) string {
	return roomID + "\x00" + userID
}

func parsePendingLeaveMember(member string) (roomID, userID string, ok bool) {
	i := -1
	for j := 0; j < len(member); j++ {
		if member[j] == 0 {
			i = j
			break
		}
	}
	if i <= 0 || i >= len(member)-1 {
		return "", "", false
	}
	return member[:i], member[i+1:], true
}

// PendingLeaveRef is a claimed due disconnect grace entry.
type PendingLeaveRef struct {
	RoomID string
	UserID string
}

// RedisConferenceStore persists conference state for multi-pod sharing.
type RedisConferenceStore struct {
	rdb redis.UniversalClient
}

func NewRedisConferenceStore(rdb redis.UniversalClient) *RedisConferenceStore {
	return &RedisConferenceStore{rdb: rdb}
}

func confRoomKey(roomID string) string    { return confRoomKeyPrefix + roomID }
func confMembersKey(roomID string) string { return confMembersKeyPref + roomID }
func confMemberKey(roomID, userID string) string {
	return confMemberKeyPref + roomID + ":" + userID
}
func confUserKey(userID string) string { return confUserKeyPrefix + userID }
func confDenyKey(roomID string) string { return confDenyKeyPrefix + roomID }
func confLockKey(roomID string) string { return confLockKeyPrefix + roomID }

func (s *RedisConferenceStore) SaveRoom(ctx context.Context, room *ConferenceRoom) error {
	data, err := json.Marshal(room)
	if err != nil {
		return err
	}
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, confRoomKey(room.RoomID), data, confRoomTTLDuration)
	pipe.SAdd(ctx, confActiveKey, room.RoomID)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *RedisConferenceStore) GetRoom(ctx context.Context, roomID string) (*ConferenceRoom, bool, error) {
	data, err := s.rdb.Get(ctx, confRoomKey(roomID)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var room ConferenceRoom
	if err := json.Unmarshal(data, &room); err != nil {
		return nil, false, err
	}
	return &room, true, nil
}

func (s *RedisConferenceStore) SaveMember(ctx context.Context, roomID string, m *ConferenceMember) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, confMemberKey(roomID, m.UserID), data, confRoomTTLDuration)
	pipe.ZAdd(ctx, confMembersKey(roomID), redis.Z{Score: float64(m.JoinTime), Member: m.UserID})
	pipe.Expire(ctx, confMembersKey(roomID), confRoomTTLDuration)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *RedisConferenceStore) GetMember(ctx context.Context, roomID, userID string) (*ConferenceMember, bool, error) {
	data, err := s.rdb.Get(ctx, confMemberKey(roomID, userID)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var m ConferenceMember
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false, err
	}
	return &m, true, nil
}

func (s *RedisConferenceStore) RemoveMember(ctx context.Context, roomID, userID string) error {
	pipe := s.rdb.Pipeline()
	pipe.Del(ctx, confMemberKey(roomID, userID))
	pipe.ZRem(ctx, confMembersKey(roomID), userID)
	pipe.Del(ctx, confUserKey(userID))
	pipe.ZRem(ctx, confPendingLeavesKey, pendingLeaveMember(roomID, userID))
	_, err := pipe.Exec(ctx)
	return err
}

// SchedulePendingLeave schedules a grace-period leave at dueUnixMilli (ZADD).
func (s *RedisConferenceStore) SchedulePendingLeave(ctx context.Context, roomID, userID string, dueUnixMilli int64) error {
	if roomID == "" || userID == "" {
		return nil
	}
	return s.rdb.ZAdd(ctx, confPendingLeavesKey, redis.Z{
		Score:  float64(dueUnixMilli),
		Member: pendingLeaveMember(roomID, userID),
	}).Err()
}

// CancelPendingLeave removes a scheduled disconnect grace leave.
func (s *RedisConferenceStore) CancelPendingLeave(ctx context.Context, roomID, userID string) error {
	if roomID == "" || userID == "" {
		return nil
	}
	return s.rdb.ZRem(ctx, confPendingLeavesKey, pendingLeaveMember(roomID, userID)).Err()
}

// CancelPendingLeavesForRoom removes all pending leaves for a room.
func (s *RedisConferenceStore) CancelPendingLeavesForRoom(ctx context.Context, roomID string) error {
	if roomID == "" {
		return nil
	}
	members, err := s.rdb.ZRange(ctx, confPendingLeavesKey, 0, -1).Result()
	if err != nil {
		if err == redis.Nil {
			return nil
		}
		return err
	}
	var toRem []any
	for _, m := range members {
		rid, _, ok := parsePendingLeaveMember(m)
		if ok && rid == roomID {
			toRem = append(toRem, m)
		}
	}
	if len(toRem) == 0 {
		return nil
	}
	return s.rdb.ZRem(ctx, confPendingLeavesKey, toRem...).Err()
}

// ClaimPendingLeave claims one scheduled leave (ZREM + SETNX). Returns false if
// already cancelled or claimed by another pod.
func (s *RedisConferenceStore) ClaimPendingLeave(ctx context.Context, roomID, userID string) (bool, error) {
	if roomID == "" || userID == "" {
		return false, nil
	}
	member := pendingLeaveMember(roomID, userID)
	claimKey := confPendingLeaveClaimPref + member
	acquired, err := s.rdb.SetNX(ctx, claimKey, "1", confPendingLeaveClaimTTL).Result()
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}
	n, err := s.rdb.ZRem(ctx, confPendingLeavesKey, member).Result()
	if err != nil {
		_ = s.rdb.Del(ctx, claimKey).Err()
		return false, err
	}
	if n == 0 {
		_ = s.rdb.Del(ctx, claimKey).Err()
		return false, nil
	}
	return true, nil
}

// ClaimDuePendingLeaves claims up to limit due entries (score <= nowUnixMilli).
// Uses SETNX so only one pod processes each member.
func (s *RedisConferenceStore) ClaimDuePendingLeaves(ctx context.Context, nowUnixMilli int64, limit int64) ([]PendingLeaveRef, error) {
	if limit <= 0 {
		limit = 32
	}
	members, err := s.rdb.ZRangeByScore(ctx, confPendingLeavesKey, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", nowUnixMilli),
		Count: limit,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	out := make([]PendingLeaveRef, 0, len(members))
	for _, member := range members {
		roomID, userID, ok := parsePendingLeaveMember(member)
		if !ok {
			_ = s.rdb.ZRem(ctx, confPendingLeavesKey, member).Err()
			continue
		}
		claimed, err := s.ClaimPendingLeave(ctx, roomID, userID)
		if err != nil || !claimed {
			continue
		}
		out = append(out, PendingLeaveRef{RoomID: roomID, UserID: userID})
	}
	return out, nil
}

func (s *RedisConferenceStore) SetUserRoom(ctx context.Context, userID, roomID string) error {
	return s.rdb.Set(ctx, confUserKey(userID), roomID, confRoomTTLDuration).Err()
}

func (s *RedisConferenceStore) GetUserRoomID(ctx context.Context, userID string) (string, bool, error) {
	roomID, err := s.rdb.Get(ctx, confUserKey(userID)).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return roomID, true, nil
}

func (s *RedisConferenceStore) ClearUserRoom(ctx context.Context, userID string) error {
	return s.rdb.Del(ctx, confUserKey(userID)).Err()
}

func (s *RedisConferenceStore) IsDenied(ctx context.Context, roomID, userID string) (bool, error) {
	return s.rdb.SIsMember(ctx, confDenyKey(roomID), userID).Result()
}

func (s *RedisConferenceStore) AddDeny(ctx context.Context, roomID, userID string) error {
	pipe := s.rdb.Pipeline()
	pipe.SAdd(ctx, confDenyKey(roomID), userID)
	pipe.Expire(ctx, confDenyKey(roomID), confRoomTTLDuration)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisConferenceStore) ClearDeny(ctx context.Context, roomID, userID string) error {
	return s.rdb.SRem(ctx, confDenyKey(roomID), userID).Err()
}

func (s *RedisConferenceStore) MemberCount(ctx context.Context, roomID string) (int64, error) {
	return s.rdb.ZCard(ctx, confMembersKey(roomID)).Result()
}

func (s *RedisConferenceStore) ListMemberIDs(ctx context.Context, roomID string, offset, count int64) ([]string, error) {
	return s.rdb.ZRange(ctx, confMembersKey(roomID), offset, offset+count-1).Result()
}

func (s *RedisConferenceStore) AllMemberIDs(ctx context.Context, roomID string) ([]string, error) {
	return s.rdb.ZRange(ctx, confMembersKey(roomID), 0, -1).Result()
}

func (s *RedisConferenceStore) ListActiveRoomIDs(ctx context.Context) ([]string, error) {
	return s.rdb.SMembers(ctx, confActiveKey).Result()
}

func (s *RedisConferenceStore) DeleteRoomAll(ctx context.Context, roomID string) error {
	ids, err := s.AllMemberIDs(ctx, roomID)
	if err != nil && err != redis.Nil {
		return err
	}
	_ = s.CancelPendingLeavesForRoom(ctx, roomID)
	pipe := s.rdb.Pipeline()
	for _, uid := range ids {
		pipe.Del(ctx, confMemberKey(roomID, uid))
		pipe.Del(ctx, confUserKey(uid))
	}
	pipe.Del(ctx, confMembersKey(roomID))
	pipe.Del(ctx, confRoomKey(roomID))
	pipe.Del(ctx, confDenyKey(roomID))
	pipe.Del(ctx, confLockKey(roomID))
	pipe.SRem(ctx, confActiveKey, roomID)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *RedisConferenceStore) WithRoomLock(ctx context.Context, roomID string, fn func(context.Context) error) error {
	key := confLockKey(roomID)
	token := randomConferenceLockToken()
	acquired, err := s.rdb.SetNX(ctx, key, token, 8*time.Second).Result()
	if err != nil {
		return err
	}
	if !acquired {
		return fmt.Errorf("conference room lock busy: %s", roomID)
	}
	defer func() {
		_ = releaseConferenceLockScript.Run(context.Background(), s.rdb, []string{key}, token).Err()
	}()
	return fn(ctx)
}

// ClaimWebhookOnce returns true if this webhook event id was not seen before (SETNX).
func (s *RedisConferenceStore) ClaimWebhookOnce(ctx context.Context, eventID string) (bool, error) {
	if eventID == "" {
		return true, nil
	}
	return s.rdb.SetNX(ctx, confWebhookKeyPrefix+eventID, "1", confWebhookTTL).Result()
}

func (s *RedisConferenceStore) BumpRev(ctx context.Context, room *ConferenceRoom) error {
	room.Rev++
	n, err := s.MemberCount(ctx, room.RoomID)
	if err == nil {
		room.RefreshParticipantCount(int(n))
	}
	return s.SaveRoom(ctx, room)
}
