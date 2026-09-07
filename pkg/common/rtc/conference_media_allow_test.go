package rtc

import "testing"

func TestMediaAllowFlagsMuteAllExemptsAdmin(t *testing.T) {
	room := &ConferenceRoom{
		RoomID:           "r1",
		AdminUserID:      "admin",
		GlobalAudioMuted: true,
		GlobalVideoOff:   false,
	}
	admin := &ConferenceMember{UserID: "admin", Role: ConferenceRoleAdmin, OnStage: true}
	cohost := &ConferenceMember{UserID: "co1", Role: ConferenceRoleCohost, OnStage: true}
	member := &ConferenceMember{UserID: "u1", Role: ConferenceRoleUser, OnStage: true}

	allowMic, allowCam := mediaAllowFlags(room, admin)
	if !allowMic {
		t.Fatal("admin should keep mic permission during mute-all")
	}
	if !allowCam {
		t.Fatal("admin camera should still be allowed when GlobalVideoOff is false")
	}

	allowMic, allowCam = mediaAllowFlags(room, cohost)
	if !allowMic {
		t.Fatal("cohost should keep mic permission during mute-all")
	}
	if !allowCam {
		t.Fatal("cohost camera should still be allowed when GlobalVideoOff is false")
	}

	allowMic, allowCam = mediaAllowFlags(room, member)
	if allowMic {
		t.Fatal("member mic should be blocked during mute-all")
	}
	if !allowCam {
		t.Fatal("member camera should still be allowed when GlobalVideoOff is false")
	}
}

func TestMediaAllowFlagsAdminStillRespectsAudioForcedOff(t *testing.T) {
	room := &ConferenceRoom{
		RoomID:           "r1",
		AdminUserID:      "admin",
		GlobalAudioMuted: true,
	}
	admin := &ConferenceMember{UserID: "admin", OnStage: true, AudioForcedOff: true}

	allowMic, _ := mediaAllowFlags(room, admin)
	if allowMic {
		t.Fatal("admin with AudioForcedOff must not get mic")
	}
}

func TestMediaAllowFlagsAudienceBlocked(t *testing.T) {
	room := &ConferenceRoom{
		RoomID:           "r1",
		AdminUserID:      "admin",
		GlobalAudioMuted: false,
	}
	audience := &ConferenceMember{UserID: "u1", OnStage: false}

	allowMic, allowCam := mediaAllowFlags(room, audience)
	if allowMic || allowCam {
		t.Fatal("audience must not publish mic/camera")
	}
}

func TestMediaAllowFlagsDisableAllVideoExemptsModerator(t *testing.T) {
	room := &ConferenceRoom{
		RoomID:         "r1",
		AdminUserID:    "admin",
		GlobalVideoOff: true,
	}
	admin := &ConferenceMember{UserID: "admin", Role: ConferenceRoleAdmin, OnStage: true}
	cohost := &ConferenceMember{UserID: "co1", Role: ConferenceRoleCohost, OnStage: true}
	member := &ConferenceMember{UserID: "u1", Role: ConferenceRoleUser, OnStage: true}

	_, allowCam := mediaAllowFlags(room, admin)
	if !allowCam {
		t.Fatal("admin camera should still be allowed during disable-all-video")
	}

	_, allowCam = mediaAllowFlags(room, cohost)
	if !allowCam {
		t.Fatal("cohost camera should still be allowed during disable-all-video")
	}

	_, allowCam = mediaAllowFlags(room, member)
	if allowCam {
		t.Fatal("member camera should be blocked during disable-all-video")
	}
}
