package rtc

import (
	"encoding/json"
	"testing"
)

// Clients treat participant metadata as a full snapshot: a lifted restriction is
// only observable if the `false` survives JSON marshalling.
func TestParticipantMetadataKeepsClearedFlags(t *testing.T) {
	mem := &ConferenceMember{
		UserID:  "u1",
		Role:    ConferenceRoleUser,
		OnStage: true,
	}
	raw := buildConferenceParticipantMetadata(mem, true, false, true, true)

	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("metadata is not valid JSON: %v (%s)", err, raw)
	}
	for _, key := range []string{"audioForcedOff", "videoForcedOff", "chatForcedOff", "cameraOn", "speakPending"} {
		v, ok := meta[key]
		if !ok {
			t.Fatalf("%s missing from metadata, cleared restrictions would stay stuck: %s", key, raw)
		}
		if v != false {
			t.Fatalf("%s = %v, want false: %s", key, v, raw)
		}
	}
	if meta["onStage"] != true {
		t.Fatalf("onStage = %v, want true: %s", meta["onStage"], raw)
	}
}

func TestParticipantMetadataCarriesForcedFlags(t *testing.T) {
	mem := &ConferenceMember{
		UserID:         "u1",
		Role:           ConferenceRoleUser,
		OnStage:        true,
		AudioForcedOff: true,
		ChatForcedOff:  true,
	}
	raw := buildConferenceParticipantMetadata(mem, true, false, false, true)

	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("metadata is not valid JSON: %v (%s)", err, raw)
	}
	if meta["audioForcedOff"] != true || meta["chatForcedOff"] != true {
		t.Fatalf("forced flags not propagated: %s", raw)
	}
	if meta["videoForcedOff"] != false {
		t.Fatalf("videoForcedOff = %v, want false: %s", meta["videoForcedOff"], raw)
	}
	if meta["allowMic"] != false || meta["allowCamera"] != true {
		t.Fatalf("publish grants mismatch: %s", raw)
	}
}
