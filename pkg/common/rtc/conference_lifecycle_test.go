package rtc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicActiveConferenceOmitsSecrets(t *testing.T) {
	room := &ConferenceRoom{
		RoomID:           "123456789",
		Title:            "standup",
		CallType:         "video",
		CreatorUserID:    "admin",
		AdminUserID:      "admin",
		ParticipantCount: 3,
		CreateTime:       1700000000,
		Locked:           true,
		PasswordHash:     "bcrypt-secret",
		InviteeUserIDs:   []string{"u1", "u2"},
		DisplayBoost:     99,
	}
	pub := PublicActiveConferenceFromRoom(room, 3)
	raw, err := json.Marshal(pub)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "bcrypt-secret") || strings.Contains(s, "passwordHash") {
		t.Fatalf("secret leaked: %s", s)
	}
	if strings.Contains(s, "inviteeUserIDs") || strings.Contains(s, "displayBoost") {
		t.Fatalf("private fields leaked: %s", s)
	}
	if !pub.HasPassword || !pub.Locked || pub.ParticipantCount != 3 {
		t.Fatalf("got %+v", pub)
	}
}

func TestPublicActiveConferenceUsesDisplayParticipantCount(t *testing.T) {
	s := &ActiveConferenceSummary{
		RoomID:               "123456789",
		Title:                "standup",
		CallType:             "video",
		CreatorUserID:        "admin",
		AdminUserID:          "admin",
		RealParticipantCount: 4,
		DisplayBoost:         99,
		ParticipantCount:     103,
	}
	pub := PublicActiveConferenceFromSummary(s)
	if pub == nil || pub.ParticipantCount != 103 {
		t.Fatalf("got %+v", pub)
	}
	raw, err := json.Marshal(pub)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "displayBoost") {
		t.Fatalf("boost leaked: %s", raw)
	}

	room := &ConferenceRoom{
		RoomID: "123456789", ParticipantCount: 103, DisplayBoost: 99,
		CallType: "video", CreatorUserID: "a", AdminUserID: "a",
	}
	fromRoom := PublicActiveConferenceFromRoom(room, 0)
	if fromRoom == nil || fromRoom.ParticipantCount != 103 {
		t.Fatalf("from room %+v", fromRoom)
	}
}

func TestListActiveConferencesSetsHasPassword(t *testing.T) {
	m, _, cleanup := newTestConferenceManager(t)
	defer cleanup()
	ctx := context.Background()
	roomID := "123123123"
	room := &ConferenceRoom{
		RoomID: roomID, AdminUserID: "admin", CreatorUserID: "admin",
		CallType: "video", Title: "locked", CreateTime: 2, PasswordHash: "hash",
	}
	if err := m.store.SaveRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	total, list, err := m.ListActiveConferences(ctx, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("total=%d len=%d", total, len(list))
	}
	if !list[0].HasPassword || list[0].Title != "locked" {
		t.Fatalf("got %+v", list[0])
	}
	pub := PublicActiveConferenceFromSummary(list[0])
	if pub == nil || !pub.HasPassword || pub.RoomID != roomID {
		t.Fatalf("public %+v", pub)
	}
}

func TestCreateAndEndBroadcastLifecycleKeys(t *testing.T) {
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
	room, _, err := m.CreateConferenceOpts(CreateConferenceOpts{
		CreatorUserID:  "admin",
		CallType:       "video",
		Title:          "standup",
		InviteeUserIDs: []string{"u1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	started := 0
	for _, ev := range tl.snapshot() {
		if strings.HasPrefix(ev, "NotifyOther:conference_started:") {
			started++
		}
	}
	if started < 1 {
		t.Fatalf("expected conference_started, got %v", tl.snapshot())
	}

	if err := m.EndConference(room.RoomID, "admin"); err != nil {
		t.Fatal(err)
	}
	closed := 0
	for _, ev := range tl.snapshot() {
		if strings.HasPrefix(ev, "NotifyOther:conference_closed:") {
			closed++
		}
	}
	if closed < 1 {
		t.Fatalf("expected conference_closed, got %v", tl.snapshot())
	}
}

func TestPickLifecycleRecipientsPrefersOnline(t *testing.T) {
	got := PickLifecycleRecipients(context.Background(),
		func(context.Context) ([]string, error) { return []string{"online-1", "online-2"}, nil },
		func(context.Context) []string { return []string{"all-1"} },
	)
	if len(got) != 2 || got[0] != "online-1" || got[1] != "online-2" {
		t.Fatalf("got %v", got)
	}
}

func TestPickLifecycleRecipientsEmptyOnlineDoesNotFallback(t *testing.T) {
	got := PickLifecycleRecipients(context.Background(),
		func(context.Context) ([]string, error) { return []string{}, nil },
		func(context.Context) []string { t.Fatal("fallback should not run"); return []string{"all"} },
	)
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestPickLifecycleRecipientsFallsBackOnOnlineError(t *testing.T) {
	got := PickLifecycleRecipients(context.Background(),
		func(context.Context) ([]string, error) { return nil, context.DeadlineExceeded },
		func(context.Context) []string { return []string{"scan-1"} },
	)
	if len(got) != 1 || got[0] != "scan-1" {
		t.Fatalf("got %v", got)
	}
}
