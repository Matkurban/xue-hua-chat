package rtc

import (
	"strings"
	"testing"
)

func TestSetCohostAndKickProtectsAdmin(t *testing.T) {
	m, cleanup := newStageTestManager(t, 9)
	defer cleanup()
	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = m.JoinConference(room.RoomID, "u1")
	_, _, _, _ = m.JoinConference(room.RoomID, "u2")

	if err := m.SetCohost(room.RoomID, "admin", "u1", true); err != nil {
		t.Fatal(err)
	}
	mem, ok := m.GetMember(room.RoomID, "u1")
	if !ok || mem.Role != ConferenceRoleCohost {
		t.Fatalf("expected cohost, got %+v ok=%v", mem, ok)
	}
	if mem.OnStage {
		t.Fatal("setting cohost should not auto-stage")
	}

	// Cohost can kick normal member.
	if err := m.Kick(room.RoomID, "u1", "u2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.GetMember(room.RoomID, "u2"); ok {
		t.Fatal("u2 should be kicked")
	}

	// Cohost cannot kick admin.
	if err := m.Kick(room.RoomID, "u1", "admin"); err == nil {
		t.Fatal("cohost should not kick admin")
	}

	// Cohost cannot remove admin.
	if err := m.Remove(room.RoomID, "u1", "admin"); err == nil {
		t.Fatal("cohost should not remove admin")
	}

	// Cohost cannot transfer admin.
	if err := m.TransferAdmin(room.RoomID, "u1", "u1"); err == nil {
		t.Fatal("cohost should not transfer admin")
	}

	// Cohost can end.
	if err := m.EndConference(room.RoomID, "u1"); err != nil {
		t.Fatal(err)
	}
}

func TestAutoTransferPrefersCohost(t *testing.T) {
	m, cleanup := newStageTestManager(t, 9)
	defer cleanup()
	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = m.JoinConference(room.RoomID, "early")
	_, _, _, _ = m.JoinConference(room.RoomID, "co")
	if err := m.SetCohost(room.RoomID, "admin", "co", true); err != nil {
		t.Fatal(err)
	}

	ended, err := m.LeaveConference(room.RoomID, "admin")
	if err != nil || ended {
		t.Fatalf("leave admin failed ended=%v err=%v", ended, err)
	}
	r, ok := m.GetConference(room.RoomID)
	if !ok {
		t.Fatal("room should remain")
	}
	if r.AdminUserID != "co" {
		t.Fatalf("expected cohost to become admin, got %q", r.AdminUserID)
	}
}

func TestRemoveAllowsRejoinKickDenies(t *testing.T) {
	m, cleanup := newStageTestManager(t, 9)
	defer cleanup()
	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = m.JoinConference(room.RoomID, "u1")
	_, _, _, _ = m.JoinConference(room.RoomID, "u2")

	if err := m.Remove(room.RoomID, "admin", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.GetMember(room.RoomID, "u1"); ok {
		t.Fatal("u1 should be removed")
	}
	if _, _, _, err := m.JoinConference(room.RoomID, "u1"); err != nil {
		t.Fatalf("removed user should rejoin, got %v", err)
	}

	if err := m.Kick(room.RoomID, "admin", "u2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.GetMember(room.RoomID, "u2"); ok {
		t.Fatal("u2 should be kicked")
	}
	_, _, _, err = m.JoinConference(room.RoomID, "u2")
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("kicked user should be denied, got %v", err)
	}

	if err := m.Remove(room.RoomID, "admin", "admin"); err == nil {
		t.Fatal("should not remove admin")
	}
	if err := m.Kick(room.RoomID, "u1", "admin"); err == nil {
		t.Fatal("should not kick admin")
	}
}
