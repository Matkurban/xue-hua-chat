package rtc

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestMeetingManager(t *testing.T) (*MeetingManager, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	m := &MeetingManager{
		store:  NewRedisMeetingStore(rdb),
		key:    "devkey",
		secret: "secret-secret-secret-secret-3232",
		url:    "ws://127.0.0.1:7880",
		stopCh: make(chan struct{}),
	}
	return m, func() {
		_ = rdb.Close()
		mr.Close()
	}
}

func seedConnectedCall(t *testing.T, m *MeetingManager, caller, callee string) *MeetingRoom {
	t.Helper()
	room, _, err := m.CreateMeeting(caller, "audio", []string{callee})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.JoinMeeting(room.RoomID, callee); err != nil {
		t.Fatal(err)
	}
	return room
}

// 被叫离开时必须整房结束：否则房间与主叫的 meeting:user key 一起悬挂，
// 主叫此后会被 /meeting/create 判为忙线、收不到任何来电。
func TestLeaveMeetingEndsOneToOneRoom(t *testing.T) {
	m, cleanup := newTestMeetingManager(t)
	defer cleanup()
	ctx := context.Background()

	room := seedConnectedCall(t, m, "caller", "callee")

	if err := m.LeaveMeeting(room.RoomID, "callee"); err != nil {
		t.Fatalf("LeaveMeeting failed: %v", err)
	}

	if _, ok, _ := m.store.GetRoom(ctx, room.RoomID); ok {
		t.Fatal("1v1 房间在参与者离开后应被整房结束")
	}
	if busy := m.GetBusyUsers([]string{"caller", "callee"}); len(busy) != 0 {
		t.Fatalf("离开后不应有忙线用户, got %v", busy)
	}
}

// 拒接时客户端也会补发 leave，此时调用者并非参与者，不能误伤房间。
func TestLeaveMeetingByNonParticipantKeepsRoom(t *testing.T) {
	m, cleanup := newTestMeetingManager(t)
	defer cleanup()
	ctx := context.Background()

	room, _, err := m.CreateMeeting("caller", "audio", []string{"callee"})
	if err != nil {
		t.Fatal(err)
	}

	if err := m.LeaveMeeting(room.RoomID, "callee"); err != nil {
		t.Fatalf("非参与者 leave 应幂等成功: %v", err)
	}

	if _, ok, _ := m.store.GetRoom(ctx, room.RoomID); !ok {
		t.Fatal("非参与者 leave 不应结束房间")
	}
	if busy := m.GetBusyUsers([]string{"caller"}); len(busy) != 1 {
		t.Fatalf("主叫应仍被判在通话中, got %v", busy)
	}
}

func TestLeaveMeetingIdempotentWhenRoomGone(t *testing.T) {
	m, cleanup := newTestMeetingManager(t)
	defer cleanup()
	ctx := context.Background()

	if err := m.store.setUserRoom(ctx, "callee", "room-gone"); err != nil {
		t.Fatal(err)
	}

	if err := m.LeaveMeeting("room-gone", "callee"); err != nil {
		t.Fatalf("房间已回收时 LeaveMeeting 应幂等成功: %v", err)
	}
	if _, ok, _ := m.store.getUserRoomID(ctx, "callee"); ok {
		t.Fatal("LeaveMeeting 应清掉调用者的忙线标记")
	}
}

func TestEndMeetingIdempotentWhenRoomGone(t *testing.T) {
	m, cleanup := newTestMeetingManager(t)
	defer cleanup()
	ctx := context.Background()

	if err := m.store.setUserRoom(ctx, "caller", "room-gone"); err != nil {
		t.Fatal(err)
	}

	if _, err := m.EndMeeting("room-gone", "caller"); err != nil {
		t.Fatalf("房间已回收时 EndMeeting 应幂等成功: %v", err)
	}
	if _, ok, _ := m.store.getUserRoomID(ctx, "caller"); ok {
		t.Fatal("EndMeeting 应清掉调用者的忙线标记")
	}
}

// meeting:user 与 meeting:room 是独立 TTL，房间先消失时忙线标记必须自愈，
// 否则该用户在标记 TTL 内一直收不到来电。
func TestGetBusyUsersClearsOrphanKey(t *testing.T) {
	m, cleanup := newTestMeetingManager(t)
	defer cleanup()
	ctx := context.Background()

	if err := m.store.setUserRoom(ctx, "ghost", "room-gone"); err != nil {
		t.Fatal(err)
	}

	if busy := m.GetBusyUsers([]string{"ghost"}); len(busy) != 0 {
		t.Fatalf("房间已不存在时不应判忙线, got %v", busy)
	}
	if _, ok, _ := m.store.getUserRoomID(ctx, "ghost"); ok {
		t.Fatal("孤儿忙线标记应被自愈删除")
	}
}

func TestRunCleanupKeepsLiveRoom(t *testing.T) {
	m, cleanup := newTestMeetingManager(t)
	defer cleanup()
	ctx := context.Background()

	room := seedConnectedCall(t, m, "caller", "callee")
	m.MarkParticipantInLiveKit(room.RoomID, "caller")
	m.MarkParticipantInLiveKit(room.RoomID, "callee")
	ageOutRoom(t, m, room.RoomID, staleRoomTimeoutSec+60)

	m.runCleanup(ctx)

	if _, ok, _ := m.store.GetRoom(ctx, room.RoomID); !ok {
		t.Fatal("进行中的长通话房间不应被清理协程删除")
	}
}

func TestRunCleanupDropsAbandonedRoom(t *testing.T) {
	m, cleanup := newTestMeetingManager(t)
	defer cleanup()
	ctx := context.Background()

	room, _, err := m.CreateMeeting("caller", "audio", []string{"callee"})
	if err != nil {
		t.Fatal(err)
	}
	ageOutRoom(t, m, room.RoomID, staleRoomTimeoutSec+60)

	m.runCleanup(ctx)

	if _, ok, _ := m.store.GetRoom(ctx, room.RoomID); ok {
		t.Fatal("无人进入 LiveKit 的废弃房间应被回收")
	}
	if busy := m.GetBusyUsers([]string{"caller"}); len(busy) != 0 {
		t.Fatalf("废弃房间回收后不应有忙线用户, got %v", busy)
	}
}

func TestRunCleanupHardTimeoutDropsLiveRoom(t *testing.T) {
	m, cleanup := newTestMeetingManager(t)
	defer cleanup()
	ctx := context.Background()

	room := seedConnectedCall(t, m, "caller", "callee")
	m.MarkParticipantInLiveKit(room.RoomID, "caller")
	m.MarkParticipantInLiveKit(room.RoomID, "callee")
	ageOutRoom(t, m, room.RoomID, hardRoomTimeoutSec+60)

	m.runCleanup(ctx)

	if _, ok, _ := m.store.GetRoom(ctx, room.RoomID); ok {
		t.Fatal("超过硬上限的房间即使有人在 LiveKit 也应被回收")
	}
}

// 把房间改成 ageSec 秒之前创建的，用于驱动清理协程分支。
func ageOutRoom(t *testing.T, m *MeetingManager, roomID string, ageSec int64) {
	t.Helper()
	ctx := context.Background()
	room, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil || !ok {
		t.Fatalf("room %s not found: %v", roomID, err)
	}
	room.CreateTime = time.Now().Unix() - ageSec
	for _, p := range room.Participants {
		p.JoinTime = room.CreateTime
	}
	if err := m.store.saveRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
}
