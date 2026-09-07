package rtc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	meetingRoomKeyPrefix = "meeting:room:"
	meetingUserKeyPrefix = "meeting:user:"
	meetingActiveKey     = "meeting:active"
	meetingLockKeyPrefix = "meeting:lock:"
	meetingRoomTTL       = 2 * time.Hour
)

// MeetingParticipant 会议参与者信息
type MeetingParticipant struct {
	UserID    string `json:"userID"`
	JoinTime  int64  `json:"joinTime"`
	InLiveKit bool   `json:"inLiveKit"`
}

// MeetingRoom 会议房间
type MeetingRoom struct {
	RoomID         string                         `json:"roomID"`
	CreatorUserID  string                         `json:"creatorUserID"`
	CallType       string                         `json:"callType"`
	InviteeUserIDs []string                       `json:"inviteeUserIDs"`
	Participants   map[string]*MeetingParticipant `json:"participants"`
	CreateTime     int64                          `json:"createTime"`
}

// RedisMeetingStore persists meeting state in Redis for multi-pod sharing.
type RedisMeetingStore struct {
	rdb redis.UniversalClient
}

func NewRedisMeetingStore(rdb redis.UniversalClient) *RedisMeetingStore {
	return &RedisMeetingStore{rdb: rdb}
}

func roomKey(roomID string) string {
	return meetingRoomKeyPrefix + roomID
}

func userKey(userID string) string {
	return meetingUserKeyPrefix + userID
}

func lockKey(roomID string) string {
	return meetingLockKeyPrefix + roomID
}

func (s *RedisMeetingStore) saveRoom(ctx context.Context, room *MeetingRoom) error {
	data, err := json.Marshal(room)
	if err != nil {
		return err
	}
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, roomKey(room.RoomID), data, meetingRoomTTL)
	pipe.SAdd(ctx, meetingActiveKey, room.RoomID)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *RedisMeetingStore) GetRoom(ctx context.Context, roomID string) (*MeetingRoom, bool, error) {
	data, err := s.rdb.Get(ctx, roomKey(roomID)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var room MeetingRoom
	if err := json.Unmarshal(data, &room); err != nil {
		return nil, false, err
	}
	return &room, true, nil
}

func (s *RedisMeetingStore) deleteRoom(ctx context.Context, room *MeetingRoom) error {
	if room == nil {
		return nil
	}
	pipe := s.rdb.Pipeline()
	for uid := range room.Participants {
		pipe.Del(ctx, userKey(uid))
	}
	pipe.Del(ctx, roomKey(room.RoomID))
	pipe.SRem(ctx, meetingActiveKey, room.RoomID)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisMeetingStore) getUserRoomID(ctx context.Context, userID string) (string, bool, error) {
	roomID, err := s.rdb.Get(ctx, userKey(userID)).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return roomID, true, nil
}

func (s *RedisMeetingStore) setUserRoom(ctx context.Context, userID, roomID string) error {
	return s.rdb.Set(ctx, userKey(userID), roomID, meetingRoomTTL).Err()
}

func (s *RedisMeetingStore) clearUserStaleRoom(ctx context.Context, userID, exceptRoomID string) error {
	existingRoomID, ok, err := s.getUserRoomID(ctx, userID)
	if err != nil || !ok || existingRoomID == exceptRoomID {
		return err
	}
	oldRoom, found, err := s.GetRoom(ctx, existingRoomID)
	if err != nil {
		return err
	}
	if found {
		return s.deleteRoom(ctx, oldRoom)
	}
	return s.rdb.Del(ctx, userKey(userID)).Err()
}

func (s *RedisMeetingStore) ListActiveRoomIDs(ctx context.Context) ([]string, error) {
	return s.rdb.SMembers(ctx, meetingActiveKey).Result()
}

func (s *RedisMeetingStore) IsParticipantActive(ctx context.Context, roomID, userID string) (bool, error) {
	room, ok, err := s.GetRoom(ctx, roomID)
	if err != nil || !ok {
		return false, err
	}
	_, exists := room.Participants[userID]
	return exists, nil
}

func (s *RedisMeetingStore) MarkParticipantInLiveKit(ctx context.Context, roomID, userID string) error {
	room, ok, err := s.GetRoom(ctx, roomID)
	if err != nil || !ok {
		return err
	}
	p, exists := room.Participants[userID]
	if !exists {
		return nil
	}
	p.InLiveKit = true
	return s.saveRoom(ctx, room)
}

func (s *RedisMeetingStore) WithRoomLock(ctx context.Context, roomID string, fn func(context.Context) error) error {
	key := lockKey(roomID)
	acquired, err := s.rdb.SetNX(ctx, key, "1", 5*time.Second).Result()
	if err != nil {
		return err
	}
	if !acquired {
		return fmt.Errorf("meeting room lock busy: %s", roomID)
	}
	defer s.rdb.Del(context.Background(), key)
	return fn(ctx)
}
