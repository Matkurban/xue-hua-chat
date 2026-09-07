package rtc

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"testing"

	"chat/pkg/common/imapi"
	"github.com/livekit/protocol/livekit"
	"github.com/openimsdk/protocol/sdkws"
)

func TestEndConferenceClearsStoreSoJoinFails(t *testing.T) {
	m, _, cleanup := newTestConferenceManager(t)
	defer cleanup()
	ctx := context.Background()
	roomID := "100000099"
	seedRoomWithMembers(t, m.store, roomID, "admin", []*ConferenceMember{
		{UserID: "admin", Role: ConferenceRoleAdmin, JoinTime: 1, InLiveKit: true},
		{UserID: "u1", Role: ConferenceRoleUser, JoinTime: 2, InLiveKit: true},
	})

	if err := m.EndConference(roomID, "admin"); err != nil {
		t.Fatal(err)
	}

	_, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("room should be deleted from store before peers can rejoin")
	}

	_, _, _, err = m.JoinConference(roomID, "u1")
	if err == nil {
		t.Fatal("join after end should fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestAdminForceEndConferenceClearsStoreWithoutHostCheck(t *testing.T) {
	m, _, cleanup := newTestConferenceManager(t)
	defer cleanup()
	ctx := context.Background()
	roomID := "100000096"
	seedRoomWithMembers(t, m.store, roomID, "admin", []*ConferenceMember{
		{UserID: "admin", Role: ConferenceRoleAdmin, JoinTime: 1, InLiveKit: true},
		{UserID: "u1", Role: ConferenceRoleUser, JoinTime: 2, InLiveKit: true},
	})

	// Platform admin path must succeed without being the room host.
	if err := m.AdminForceEndConference(roomID); err != nil {
		t.Fatal(err)
	}

	_, ok, err := m.store.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("room should be deleted after admin force end")
	}

	// Idempotent: ending a missing room is a no-op.
	if err := m.AdminForceEndConference(roomID); err != nil {
		t.Fatal(err)
	}
}

func TestLeaveLastMemberEndsAndJoinFails(t *testing.T) {
	m, _, cleanup := newTestConferenceManager(t)
	defer cleanup()
	roomID := "100000098"
	seedRoomWithMembers(t, m.store, roomID, "admin", []*ConferenceMember{
		{UserID: "admin", Role: ConferenceRoleAdmin, JoinTime: 1, InLiveKit: true},
	})

	ended, err := m.LeaveConference(roomID, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !ended {
		t.Fatal("last member leave should end the room")
	}

	_, _, _, err = m.JoinConference(roomID, "u1")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("expected not found after room ended, got %v", err)
	}
}

func TestEndConferenceNotifiesBeforeDeleteRoom(t *testing.T) {
	store, _, cleanup := newTestConferenceStore(t)
	defer cleanup()

	tl := &endTimeline{}
	lk := &recordingConferenceLK{tl: tl}
	im := &recordingConferenceIM{tl: tl}
	m := &ConferenceManager{
		store:       store,
		lk:          lk,
		im:          im,
		adminUserID: "imAdmin",
		stopCh:      make(chan struct{}),
	}

	roomID := "100000097"
	seedRoomWithMembers(t, m.store, roomID, "admin", []*ConferenceMember{
		{UserID: "admin", Role: ConferenceRoleAdmin, JoinTime: 1, InLiveKit: true},
		{UserID: "u1", Role: ConferenceRoleUser, JoinTime: 2, InLiveKit: true},
		{UserID: "u2", Role: ConferenceRoleUser, JoinTime: 3, InLiveKit: true},
	})

	if err := m.EndConference(roomID, "admin"); err != nil {
		t.Fatal(err)
	}

	events := tl.snapshot()
	deleteIdx := -1
	firstEndedIdx := -1
	endedRecipients := map[string]bool{}
	for i, ev := range events {
		switch {
		case strings.HasPrefix(ev, "DeleteRoom:"):
			if deleteIdx < 0 {
				deleteIdx = i
			}
		case strings.HasPrefix(ev, "NotifyEnded:"):
			if firstEndedIdx < 0 {
				firstEndedIdx = i
			}
			uid := strings.TrimPrefix(ev, "NotifyEnded:")
			endedRecipients[uid] = true
		}
	}
	if firstEndedIdx < 0 {
		t.Fatalf("expected ended notifies; timeline=%v", events)
	}
	if deleteIdx < 0 {
		t.Fatalf("expected DeleteRoom; timeline=%v", events)
	}
	if firstEndedIdx >= deleteIdx {
		t.Fatalf("ended notify must precede DeleteRoom; timeline=%v", events)
	}
	for i, ev := range events {
		if strings.HasPrefix(ev, "NotifyEnded:") && i >= deleteIdx {
			t.Fatalf("ended notify %q at %d after DeleteRoom %d; timeline=%v", ev, i, deleteIdx, events)
		}
	}
	for _, uid := range []string{"admin", "u1", "u2"} {
		if !endedRecipients[uid] {
			t.Fatalf("member %s missing ended notify; got %v timeline=%v", uid, endedRecipients, events)
		}
	}
}

type endTimeline struct {
	mu     sync.Mutex
	events []string
}

func (t *endTimeline) add(ev string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, ev)
}

func (t *endTimeline) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.events))
	copy(out, t.events)
	return out
}

type recordingConferenceLK struct {
	tl *endTimeline
}

func (f *recordingConferenceLK) ClientURL() string { return "ws://test" }
func (f *recordingConferenceLK) APIKey() string    { return "k" }
func (f *recordingConferenceLK) APISecret() string { return "s" }

func (f *recordingConferenceLK) CreateRoom(context.Context, string, uint32) error { return nil }
func (f *recordingConferenceLK) DeleteRoom(_ context.Context, roomID string) error {
	f.tl.add("DeleteRoom:" + roomID)
	return nil
}
func (f *recordingConferenceLK) RemoveParticipant(context.Context, string, string) error { return nil }
func (f *recordingConferenceLK) SendData(_ context.Context, roomID string, payload []byte) error {
	f.tl.add("SendData:" + roomID)
	_ = payload
	return nil
}
func (f *recordingConferenceLK) UpdateParticipantPermission(
	context.Context, string, string, bool, bool, bool, bool, bool, *ConferenceMember,
) error {
	return nil
}
func (f *recordingConferenceLK) MuteMediaKind(context.Context, string, string, bool, bool) error {
	return nil
}
func (f *recordingConferenceLK) StartRoomCompositeEgress(context.Context, string, string) (string, error) {
	return "", nil
}
func (f *recordingConferenceLK) StopEgress(context.Context, string) error { return nil }
func (f *recordingConferenceLK) CreateRTMPIngress(context.Context, string, string) (*livekit.IngressInfo, error) {
	return nil, nil
}
func (f *recordingConferenceLK) CreateWHIPIngress(context.Context, string, string) (*livekit.IngressInfo, error) {
	return nil, nil
}
func (f *recordingConferenceLK) DeleteIngress(context.Context, string) error { return nil }
func (f *recordingConferenceLK) GenerateToken(string, string, string, bool, bool, bool) (string, error) {
	return "tok", nil
}

type recordingConferenceIM struct {
	imapi.CallerInterface
	tl *endTimeline
}

func (r *recordingConferenceIM) ImAdminTokenWithDefaultAdmin(context.Context) (string, error) {
	return "test-token", nil
}

func (r *recordingConferenceIM) GetUserInfo(context.Context, string) (*sdkws.UserInfo, error) {
	return nil, nil
}

func (r *recordingConferenceIM) SendBusinessNotification(_ context.Context, key, data, _, recvUserID string) error {
	if key != conferenceSyncBusinessID {
		r.tl.add("NotifyOther:" + key + ":" + recvUserID)
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		r.tl.add("NotifyBad:" + recvUserID)
		return nil
	}
	payload, err := UnmarshalConferenceSyncProto(raw)
	if err != nil {
		r.tl.add("NotifyBad:" + recvUserID)
		return nil
	}
	if payload.Event == "ended" {
		r.tl.add("NotifyEnded:" + recvUserID)
		return nil
	}
	r.tl.add("Notify:" + payload.Event + ":" + recvUserID)
	return nil
}
