package rtc

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/livekit/protocol/livekit"
	"github.com/redis/go-redis/v9"
)

func newTestConferenceStore(t *testing.T) (*RedisConferenceStore, *miniredis.Miniredis, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRedisConferenceStore(rdb), mr, func() {
		_ = rdb.Close()
		mr.Close()
	}
}

func newTestConferenceManager(t *testing.T) (*ConferenceManager, *miniredis.Miniredis, func()) {
	t.Helper()
	store, mr, cleanup := newTestConferenceStore(t)
	lk, err := newConferenceLK("http://127.0.0.1:1", "devkey", "secret")
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	m := &ConferenceManager{
		store:  store,
		lk:     lk,
		stopCh: make(chan struct{}),
	}
	return m, mr, cleanup
}

func seedRoomWithMembers(t *testing.T, store *RedisConferenceStore, roomID, adminID string, members []*ConferenceMember) {
	t.Helper()
	ctx := context.Background()
	room := &ConferenceRoom{
		RoomID: roomID, AdminUserID: adminID, CreatorUserID: adminID,
		CallType: "video", CreateTime: time.Now().Unix(), ParticipantCount: len(members),
	}
	if err := store.SaveRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	for _, mem := range members {
		if err := store.SaveMember(ctx, roomID, mem); err != nil {
			t.Fatal(err)
		}
		_ = store.SetUserRoom(ctx, mem.UserID, roomID)
	}
}

func TestParsePendingLeaveMember(t *testing.T) {
	rid, uid, ok := parsePendingLeaveMember("room1\x00user2")
	if !ok || rid != "room1" || uid != "user2" {
		t.Fatalf("got %q %q %v", rid, uid, ok)
	}
	if _, _, ok := parsePendingLeaveMember("noulid"); ok {
		t.Fatal("expected fail")
	}
}

func TestPendingLeaveScheduleCancelClaim(t *testing.T) {
	store, _, cleanup := newTestConferenceStore(t)
	defer cleanup()
	ctx := context.Background()

	due := time.Now().Add(conferenceDisconnectGrace).UnixMilli()
	if err := store.SchedulePendingLeave(ctx, "r1", "u1", due); err != nil {
		t.Fatal(err)
	}
	if err := store.CancelPendingLeave(ctx, "r1", "u1"); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimPendingLeave(ctx, "r1", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("cancelled leave should not be claimable")
	}

	past := time.Now().Add(-time.Second).UnixMilli()
	if err := store.SchedulePendingLeave(ctx, "r1", "u2", past); err != nil {
		t.Fatal(err)
	}
	refs, err := store.ClaimDuePendingLeaves(ctx, time.Now().UnixMilli(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].RoomID != "r1" || refs[0].UserID != "u2" {
		t.Fatalf("refs=%v", refs)
	}
	// Second claim of same due batch is empty.
	refs2, err := store.ClaimDuePendingLeaves(ctx, time.Now().UnixMilli(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs2) != 0 {
		t.Fatalf("expected empty, got %v", refs2)
	}
}

func TestDisconnectGraceRejoinCancelsLeave(t *testing.T) {
	m, _, cleanup := newTestConferenceManager(t)
	defer cleanup()
	ctx := context.Background()
	roomID := "100000001"
	seedRoomWithMembers(t, m.store, roomID, "admin", []*ConferenceMember{
		{UserID: "admin", Role: ConferenceRoleAdmin, JoinTime: 1, InLiveKit: true},
		{UserID: "u1", Role: ConferenceRoleUser, JoinTime: 2, InLiveKit: true},
	})

	m.handleWebhook(ctx, &livekit.WebhookEvent{
		Id: "ev-left-1", Event: "participant_left",
		Room:        &livekit.Room{Name: roomID},
		Participant: &livekit.ParticipantInfo{Identity: "u1"},
	})
	mem, ok, err := m.store.GetMember(ctx, roomID, "u1")
	if err != nil || !ok || mem.InLiveKit {
		t.Fatalf("expected offline member, ok=%v inLK=%v err=%v", ok, mem != nil && mem.InLiveKit, err)
	}

	m.handleWebhook(ctx, &livekit.WebhookEvent{
		Id: "ev-join-1", Event: "participant_joined",
		Room:        &livekit.Room{Name: roomID},
		Participant: &livekit.ParticipantInfo{Identity: "u1"},
	})
	mem, ok, err = m.store.GetMember(ctx, roomID, "u1")
	if err != nil || !ok || !mem.InLiveKit {
		t.Fatalf("expected back online, ok=%v inLK=%v err=%v", ok, mem != nil && mem.InLiveKit, err)
	}

	// Even if drain runs, cancelled pending leave must not remove the member.
	m.drainPendingLeaves(ctx)
	mem, ok, err = m.store.GetMember(ctx, roomID, "u1")
	if err != nil || !ok {
		t.Fatalf("rejoin should cancel leave, ok=%v err=%v", ok, err)
	}
}

func TestDisconnectGraceTimeoutLeavesAndTransfersAdmin(t *testing.T) {
	m, _, cleanup := newTestConferenceManager(t)
	defer cleanup()
	ctx := context.Background()
	roomID := "100000002"
	seedRoomWithMembers(t, m.store, roomID, "admin", []*ConferenceMember{
		{UserID: "admin", Role: ConferenceRoleAdmin, JoinTime: 10, InLiveKit: true},
		{UserID: "early", Role: ConferenceRoleUser, JoinTime: 1, InLiveKit: true},
		{UserID: "late", Role: ConferenceRoleUser, JoinTime: 20, InLiveKit: true},
	})

	m.handleWebhook(ctx, &livekit.WebhookEvent{
		Id: "ev-left-admin", Event: "participant_left",
		Room:        &livekit.Room{Name: roomID},
		Participant: &livekit.ParticipantInfo{Identity: "admin"},
	})

	// Simulate due time: claim + apply (same as AfterFunc / drain).
	duePast := time.Now().Add(-time.Second).UnixMilli()
	_ = m.store.CancelPendingLeave(ctx, roomID, "admin")
	if err := m.store.SchedulePendingLeave(ctx, roomID, "admin", duePast); err != nil {
		t.Fatal(err)
	}
	m.drainPendingLeaves(ctx)

	_, ok, err := m.store.GetMember(ctx, roomID, "admin")
	if err != nil || ok {
		t.Fatalf("admin should be removed, ok=%v err=%v", ok, err)
	}
	room, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil || !ok {
		t.Fatalf("room should remain, ok=%v err=%v", ok, err)
	}
	if room.AdminUserID != "early" {
		t.Fatalf("admin transferred to earliest joiner, got %q", room.AdminUserID)
	}
}

func TestDisconnectGraceNoopAfterIntentionalLeave(t *testing.T) {
	m, _, cleanup := newTestConferenceManager(t)
	defer cleanup()
	ctx := context.Background()
	roomID := "100000003"
	seedRoomWithMembers(t, m.store, roomID, "admin", []*ConferenceMember{
		{UserID: "admin", Role: ConferenceRoleAdmin, JoinTime: 1, InLiveKit: true},
		{UserID: "u1", Role: ConferenceRoleUser, JoinTime: 2, InLiveKit: true},
	})

	if _, err := m.LeaveConference(roomID, "u1"); err != nil {
		t.Fatal(err)
	}
	m.handleWebhook(ctx, &livekit.WebhookEvent{
		Id: "ev-left-after-leave", Event: "participant_left",
		Room:        &livekit.Room{Name: roomID},
		Participant: &livekit.ParticipantInfo{Identity: "u1"},
	})
	n, err := m.store.MemberCount(ctx, roomID)
	if err != nil || n != 1 {
		t.Fatalf("memberCount=%d err=%v", n, err)
	}
	refs, err := m.store.ClaimDuePendingLeaves(ctx, time.Now().Add(time.Hour).UnixMilli(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("no pending leave expected, got %v", refs)
	}
}

func TestRemoveMemberCancelsPendingLeave(t *testing.T) {
	store, _, cleanup := newTestConferenceStore(t)
	defer cleanup()
	ctx := context.Background()
	_ = store.SchedulePendingLeave(ctx, "r", "u", time.Now().UnixMilli())
	if err := store.RemoveMember(ctx, "r", "u"); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimPendingLeave(ctx, "r", "u")
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("RemoveMember should cancel pending leave")
	}
}
