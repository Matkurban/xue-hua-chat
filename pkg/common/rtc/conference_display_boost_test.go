package rtc

import (
	"context"
	"strings"
	"testing"

	constantpb "github.com/openimsdk/protocol/constant"
)

func TestRefreshParticipantCount(t *testing.T) {
	room := &ConferenceRoom{DisplayBoost: 10}
	room.RefreshParticipantCount(5)
	if room.ParticipantCount != 15 {
		t.Fatalf("got %d want 15", room.ParticipantCount)
	}
	room.DisplayBoost = -3
	room.RefreshParticipantCount(5)
	if room.DisplayBoost != 0 || room.ParticipantCount != 5 {
		t.Fatalf("boost=%d count=%d", room.DisplayBoost, room.ParticipantCount)
	}
}

func TestAdjustDisplayBoostNotifiesMembers(t *testing.T) {
	store, _, cleanup := newTestConferenceStore(t)
	defer cleanup()

	tl := &endTimeline{}
	im := &recordingConferenceIM{tl: tl}
	m := &ConferenceManager{
		store:       store,
		lk:          &recordingConferenceLK{tl: tl},
		im:          im,
		adminUserID: "imAdmin",
		stopCh:      make(chan struct{}),
	}

	roomID := "270956076"
	seedRoomWithMembers(t, m.store, roomID, "admin", []*ConferenceMember{
		{UserID: "admin", Role: ConferenceRoleAdmin, JoinTime: 1, InLiveKit: true},
		{UserID: "u1", Role: ConferenceRoleUser, JoinTime: 2, InLiveKit: true},
	})

	ctx := context.WithValue(context.Background(), constantpb.OperationID, "test-adjust-boost")
	summary, err := m.AdjustDisplayBoost(ctx, roomID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if summary.DisplayBoost != 10 {
		t.Fatalf("boost=%d want 10", summary.DisplayBoost)
	}
	if summary.RealParticipantCount != 2 {
		t.Fatalf("real=%d want 2", summary.RealParticipantCount)
	}
	if summary.ParticipantCount != 12 {
		t.Fatalf("display=%d want 12", summary.ParticipantCount)
	}
	if summary.NotifiedCount != 2 || summary.NotifyFailed != 0 {
		t.Fatalf("notified=%d failed=%d", summary.NotifiedCount, summary.NotifyFailed)
	}

	events := tl.snapshot()
	gotAdmin, gotU1 := false, false
	for _, ev := range events {
		if strings.Contains(ev, "Notify:count_adjusted:admin") {
			gotAdmin = true
		}
		if strings.Contains(ev, "Notify:count_adjusted:u1") {
			gotU1 = true
		}
	}
	if !gotAdmin || !gotU1 {
		t.Fatalf("missing count_adjusted notify, events=%v", events)
	}

	// Negative delta should clamp at 0 and still notify.
	summary, err = m.AdjustDisplayBoost(ctx, roomID, -100)
	if err != nil {
		t.Fatal(err)
	}
	if summary.DisplayBoost != 0 || summary.ParticipantCount != 2 {
		t.Fatalf("after reduce boost=%d count=%d", summary.DisplayBoost, summary.ParticipantCount)
	}
	if summary.NotifiedCount != 2 {
		t.Fatalf("reduce notified=%d", summary.NotifiedCount)
	}
}

func TestCtxWithOperationIDFillsMissing(t *testing.T) {
	ctx := ctxWithOperationID(context.Background())
	op, _ := ctx.Value(constantpb.OperationID).(string)
	if op == "" {
		t.Fatal("expected generated operationID")
	}
	if !strings.HasPrefix(op, "conf-sync-") {
		t.Fatalf("unexpected operationID %q", op)
	}

	kept := context.WithValue(context.Background(), constantpb.OperationID, "keep-me")
	got := ctxWithOperationID(kept)
	if v, _ := got.Value(constantpb.OperationID).(string); v != "keep-me" {
		t.Fatalf("got %q want keep-me", v)
	}
}

func TestAdjustDisplayBoostNotifiesWithoutCallerOperationID(t *testing.T) {
	store, _, cleanup := newTestConferenceStore(t)
	defer cleanup()

	tl := &endTimeline{}
	im := &recordingConferenceIM{tl: tl}
	m := &ConferenceManager{
		store:       store,
		lk:          &recordingConferenceLK{tl: tl},
		im:          im,
		adminUserID: "imAdmin",
		stopCh:      make(chan struct{}),
	}

	roomID := "270956077"
	seedRoomWithMembers(t, m.store, roomID, "admin", []*ConferenceMember{
		{UserID: "admin", Role: ConferenceRoleAdmin, JoinTime: 1, InLiveKit: true},
		{UserID: "u1", Role: ConferenceRoleUser, JoinTime: 2, InLiveKit: true},
	})

	// Background has no operationID; broadcastSync must inject one and still notify.
	summary, err := m.AdjustDisplayBoost(context.Background(), roomID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if summary.NotifiedCount != 2 || summary.NotifyFailed != 0 {
		t.Fatalf("notified=%d failed=%d", summary.NotifiedCount, summary.NotifyFailed)
	}
}
