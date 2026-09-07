package rtc

import (
	"bytes"
	"testing"
)

func TestConferenceSyncProtoRoundTrip(t *testing.T) {
	in := &ConferenceSyncPayload{
		RoomID: "r1", Rev: 42, AdminUserID: "h1", ParticipantCount: 7,
		GlobalAudioMuted: true, ChatMuted: true, Locked: true,
		PinnedUserID: "p1", Event: "kicked", TargetUserID: "t1",
		StageStateJSON: `stage-ok`,
	}
	raw := MarshalConferenceSyncProto(in)
	out, err := UnmarshalConferenceSyncProto(raw)
	if err != nil {
		t.Fatal(err)
	}
	if out.RoomID != in.RoomID || out.Rev != in.Rev || out.AdminUserID != in.AdminUserID ||
		out.ParticipantCount != in.ParticipantCount || out.GlobalAudioMuted != in.GlobalAudioMuted ||
		out.ChatMuted != in.ChatMuted || out.Locked != in.Locked ||
		out.PinnedUserID != in.PinnedUserID || out.Event != in.Event || out.TargetUserID != in.TargetUserID ||
		out.StageStateJSON != in.StageStateJSON {
		t.Fatalf("mismatch: %+v vs %+v", in, out)
	}
	if bytes.Contains(raw, []byte("{")) {
		t.Fatal("expected binary protobuf, got JSON-looking payload")
	}
}

func TestConferenceSyncProtoChatRevokeAndMuteFields(t *testing.T) {
	in := &ConferenceSyncPayload{
		RoomID:              "r2",
		Rev:                 9,
		AdminUserID:         "admin1",
		ParticipantCount:    3,
		Event:               "chat_revoke",
		TargetUserID:        "sender1",
		TargetChatForcedOff: true,
		RevokeTargetSeq:     123456789,
	}
	raw := MarshalConferenceSyncProto(in)
	out, err := UnmarshalConferenceSyncProto(raw)
	if err != nil {
		t.Fatal(err)
	}
	if out.Event != "chat_revoke" || out.TargetUserID != "sender1" ||
		out.RevokeTargetSeq != 123456789 || !out.TargetChatForcedOff {
		t.Fatalf("chat_revoke/mute fields mismatch: %+v", out)
	}
	if bytes.Contains(raw, []byte("{")) {
		t.Fatal("expected binary protobuf, got JSON-looking payload")
	}
}
