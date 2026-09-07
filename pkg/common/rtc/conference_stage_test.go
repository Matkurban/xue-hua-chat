package rtc

import (
	"testing"
	"time"
)

func newStageTestManager(t *testing.T, autoStage int) (*ConferenceManager, func()) {
	t.Helper()
	m, _, cleanup := newTestConferenceManager(t)
	m.autoStageLimit = normalizeStageLimit(autoStage, DefaultAutoStageLimit)
	return m, cleanup
}

func mustJoinAudience(t *testing.T, m *ConferenceManager, roomID, userID string) *ConferenceMember {
	t.Helper()
	_, _, mem, err := m.JoinConference(roomID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if mem.OnStage {
		t.Fatalf("%s should join as audience", userID)
	}
	return mem
}

func mustApproveSpeak(t *testing.T, m *ConferenceManager, roomID, hostID, target string) {
	t.Helper()
	if err := m.ApplySpeak(roomID, target); err != nil {
		t.Fatal(err)
	}
	if err := m.ApproveSpeak(roomID, hostID, target, true); err != nil {
		t.Fatal(err)
	}
	mem, ok := m.GetMember(roomID, target)
	if !ok || !mem.OnStage || mem.SpeakPending {
		t.Fatalf("expected %s on stage after approve", target)
	}
}

func TestJoinOnlyAdminOnStage(t *testing.T) {
	m, cleanup := newStageTestManager(t, 9)
	defer cleanup()

	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	admin, ok := m.GetMember(room.RoomID, "admin")
	if !ok || !admin.OnStage {
		t.Fatal("admin should be on stage")
	}
	if room.LinkMicDisabled {
		t.Fatal("link mic should default on")
	}

	mustJoinAudience(t, m, room.RoomID, "u1")
	mustJoinAudience(t, m, room.RoomID, "u2")
}

func TestStageSlotFullBlocksApplyAndApprove(t *testing.T) {
	m, cleanup := newStageTestManager(t, 1)
	defer cleanup()

	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mustJoinAudience(t, m, room.RoomID, "u1")
	err = m.ApplySpeak(room.RoomID, "u1")
	if err == nil || err.Error() != "stage_slot_full" {
		t.Fatalf("expected stage_slot_full on apply, got %v", err)
	}
}

func TestApproveSpeakAfterLeaveFreesSlot(t *testing.T) {
	m, cleanup := newStageTestManager(t, 2)
	defer cleanup()

	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mustJoinAudience(t, m, room.RoomID, "u1")
	mustApproveSpeak(t, m, room.RoomID, "admin", "u1")
	mustJoinAudience(t, m, room.RoomID, "u2")
	if err := m.ApplySpeak(room.RoomID, "u2"); err == nil || err.Error() != "stage_slot_full" {
		t.Fatalf("expected stage_slot_full before leave, got %v", err)
	}
	if err := m.LeaveSpeak(room.RoomID, "u1", ""); err != nil {
		t.Fatal(err)
	}
	mustApproveSpeak(t, m, room.RoomID, "admin", "u2")
}

func TestOnStageMembersCanAllOpenCamera(t *testing.T) {
	m, cleanup := newStageTestManager(t, 3)
	defer cleanup()

	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, uid := range []string{"u1", "u2"} {
		mustJoinAudience(t, m, room.RoomID, uid)
		mustApproveSpeak(t, m, room.RoomID, "admin", uid)
	}
	for _, uid := range []string{"admin", "u1", "u2"} {
		if err := m.ToggleCamera(room.RoomID, uid, true); err != nil {
			t.Fatalf("toggle camera %s: %v", uid, err)
		}
	}
	r, ok := m.GetConference(room.RoomID)
	if !ok || len(r.CameraOnUserIDs) != 3 {
		t.Fatalf("camera list=%v", r.CameraOnUserIDs)
	}
}

func TestLeaveSpeakFreesCamera(t *testing.T) {
	m, cleanup := newStageTestManager(t, 2)
	defer cleanup()

	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mustJoinAudience(t, m, room.RoomID, "u1")
	mustApproveSpeak(t, m, room.RoomID, "admin", "u1")
	if err := m.ToggleCamera(room.RoomID, "u1", true); err != nil {
		t.Fatal(err)
	}
	if err := m.LeaveSpeak(room.RoomID, "u1", ""); err != nil {
		t.Fatal(err)
	}
	mem, ok := m.GetMember(room.RoomID, "u1")
	if !ok || mem.OnStage || mem.CameraOn {
		t.Fatal("expected audience after leave speak")
	}
	r, _ := m.GetConference(room.RoomID)
	if containsUserID(r.CameraOnUserIDs, "u1") {
		t.Fatal("camera slot not freed")
	}
	if err := m.LeaveSpeak(room.RoomID, "admin", ""); err == nil {
		t.Fatal("admin should not leave stage")
	}
}

func TestForceLeaveSpeak(t *testing.T) {
	m, cleanup := newStageTestManager(t, 9)
	defer cleanup()
	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mustJoinAudience(t, m, room.RoomID, "u1")
	mustJoinAudience(t, m, room.RoomID, "u2")
	mustApproveSpeak(t, m, room.RoomID, "admin", "u1")
	if err := m.SetCohost(room.RoomID, "admin", "u2", true); err != nil {
		t.Fatal(err)
	}

	if err := m.LeaveSpeak(room.RoomID, "u2", "u1"); err != nil {
		t.Fatal(err)
	}
	mem, _ := m.GetMember(room.RoomID, "u1")
	if mem.OnStage {
		t.Fatal("cohost should force u1 off stage")
	}

	mustApproveSpeak(t, m, room.RoomID, "admin", "u1")
	if err := m.LeaveSpeak(room.RoomID, "u1", "u2"); err == nil {
		t.Fatal("regular member should not force others off")
	}
	if err := m.LeaveSpeak(room.RoomID, "admin", "admin"); err == nil {
		t.Fatal("admin cannot be forced off")
	}
}

func TestCohostMustRaiseHand(t *testing.T) {
	m, cleanup := newStageTestManager(t, 9)
	defer cleanup()
	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mustJoinAudience(t, m, room.RoomID, "co")
	if err := m.SetCohost(room.RoomID, "admin", "co", true); err != nil {
		t.Fatal(err)
	}
	mem, ok := m.GetMember(room.RoomID, "co")
	if !ok || mem.Role != ConferenceRoleCohost || mem.OnStage {
		t.Fatalf("cohost should stay audience until approved, got %+v", mem)
	}
	mustApproveSpeak(t, m, room.RoomID, "admin", "co")
}

func TestLinkMicToggle(t *testing.T) {
	m, cleanup := newStageTestManager(t, 9)
	defer cleanup()
	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mustJoinAudience(t, m, room.RoomID, "u1")
	mustApproveSpeak(t, m, room.RoomID, "admin", "u1")
	mustJoinAudience(t, m, room.RoomID, "u2")
	if err := m.ApplySpeak(room.RoomID, "u2"); err != nil {
		t.Fatal(err)
	}

	if err := m.SetLinkMic(room.RoomID, "admin", false); err != nil {
		t.Fatal(err)
	}
	r, _ := m.GetConference(room.RoomID)
	if !r.LinkMicDisabled {
		t.Fatal("expected link mic disabled")
	}
	u1, _ := m.GetMember(room.RoomID, "u1")
	if !u1.OnStage {
		t.Fatal("existing speaker should stay on stage")
	}
	u2, _ := m.GetMember(room.RoomID, "u2")
	if u2.SpeakPending {
		t.Fatal("pending apply should be cleared")
	}
	if err := m.ApplySpeak(room.RoomID, "u2"); err == nil || err.Error() != "link_mic_disabled" {
		t.Fatalf("expected link_mic_disabled on apply, got %v", err)
	}
	if err := m.ApproveSpeak(room.RoomID, "admin", "u2", true); err == nil || err.Error() != "link_mic_disabled" {
		t.Fatalf("expected link_mic_disabled on approve, got %v", err)
	}
	if err := m.SetLinkMic(room.RoomID, "u1", false); err == nil {
		t.Fatal("non-admin should not toggle link mic")
	}

	mustJoinAudience(t, m, room.RoomID, "co")
	if err := m.SetCohost(room.RoomID, "admin", "co", true); err != nil {
		t.Fatal(err)
	}
	if err := m.SetLinkMic(room.RoomID, "co", true); err != nil {
		t.Fatal(err)
	}
	r, _ = m.GetConference(room.RoomID)
	if r.LinkMicDisabled {
		t.Fatal("cohost should be able to enable link mic")
	}
	if err := m.SetLinkMic(room.RoomID, "co", false); err != nil {
		t.Fatal(err)
	}
	r, _ = m.GetConference(room.RoomID)
	if !r.LinkMicDisabled {
		t.Fatal("cohost should be able to disable link mic")
	}

	if err := m.SetLinkMic(room.RoomID, "admin", true); err != nil {
		t.Fatal(err)
	}
	mustApproveSpeak(t, m, room.RoomID, "admin", "u2")
}

func TestRejectSpeak(t *testing.T) {
	m, cleanup := newStageTestManager(t, 2)
	defer cleanup()
	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mustJoinAudience(t, m, room.RoomID, "u1")
	if err := m.ApplySpeak(room.RoomID, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := m.ApproveSpeak(room.RoomID, "admin", "u1", false); err != nil {
		t.Fatal(err)
	}
	mem, _ := m.GetMember(room.RoomID, "u1")
	if mem.OnStage || mem.SpeakPending {
		t.Fatal("expected rejected audience")
	}
	_ = time.Now()
}

func TestToggleCameraGlobalVideoOffExemptsModerator(t *testing.T) {
	m, cleanup := newStageTestManager(t, 2)
	defer cleanup()

	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mustJoinAudience(t, m, room.RoomID, "u1")
	mustApproveSpeak(t, m, room.RoomID, "admin", "u1")
	video := false
	if err := m.SetMediaAll(room.RoomID, "admin", nil, &video); err != nil {
		t.Fatal(err)
	}
	if err := m.ToggleCamera(room.RoomID, "admin", true); err != nil {
		t.Fatalf("admin should open camera during disable-all-video: %v", err)
	}
	err = m.ToggleCamera(room.RoomID, "u1", true)
	if err == nil || err.Error() != "camera disabled by policy" {
		t.Fatalf("member should be blocked, got %v", err)
	}
}
