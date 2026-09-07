package rtc

import (
	"strings"
	"testing"

	"github.com/livekit/protocol/livekit"
)

func TestIsEgressTerminal(t *testing.T) {
	cases := []struct {
		status livekit.EgressStatus
		want   bool
	}{
		{livekit.EgressStatus_EGRESS_STARTING, false},
		{livekit.EgressStatus_EGRESS_ACTIVE, false},
		{livekit.EgressStatus_EGRESS_ENDING, false},
		{livekit.EgressStatus_EGRESS_COMPLETE, true},
		{livekit.EgressStatus_EGRESS_FAILED, true},
		{livekit.EgressStatus_EGRESS_ABORTED, true},
		{livekit.EgressStatus_EGRESS_LIMIT_REACHED, true},
	}
	for _, tc := range cases {
		if got := isEgressTerminal(tc.status); got != tc.want {
			t.Fatalf("isEgressTerminal(%v)=%v want %v", tc.status, got, tc.want)
		}
	}
}

func TestNormalizeIngressInputType(t *testing.T) {
	if got := normalizeIngressInputType("whip"); got != "whip" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeIngressInputType("WHIP"); got != "whip" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeIngressInputType(""); got != "rtmp" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeIngressInputType("rtmp"); got != "rtmp" {
		t.Fatalf("got %q", got)
	}
}

func TestPublishSources(t *testing.T) {
	if got := publishSources(true, true); got != nil {
		t.Fatalf("unrestricted want nil, got %v", got)
	}
	micOnly := publishSources(true, false)
	if len(micOnly) != 2 {
		t.Fatalf("mic-only len=%d", len(micOnly))
	}
	camOnly := publishSources(false, true)
	if len(camOnly) != 2 {
		t.Fatalf("cam-only len=%d", len(camOnly))
	}
	none := publishSources(false, false)
	if none == nil || len(none) != 0 {
		t.Fatalf("both-off want empty non-nil, got %v", none)
	}
}

func TestNormalizeCreateInvitees(t *testing.T) {
	inv := normalizeCreateInvitees("admin", []string{"a", "b", "a", "admin"})
	if len(inv) != 2 || inv[0] != "a" || inv[1] != "b" {
		t.Fatalf("invitees=%v", inv)
	}
}

func TestBuildConferenceParticipantMetadata(t *testing.T) {
	mem := &ConferenceMember{
		UserID: "u1", Role: ConferenceRoleUser, AudioForcedOff: true,
	}
	raw := buildConferenceParticipantMetadata(mem, true, false, false, true)
	if raw == "" || !strings.Contains(raw, `"role":"user"`) || !strings.Contains(raw, `"audioForcedOff":true`) {
		t.Fatalf("meta=%s", raw)
	}
	if strings.Contains(raw, "handRaised") {
		t.Fatalf("unexpected handRaised in meta=%s", raw)
	}
}

func TestIsIngressTerminal(t *testing.T) {
	if isIngressTerminal(nil) != true {
		t.Fatal("nil should be terminal")
	}
	info := &livekit.IngressInfo{State: &livekit.IngressState{Status: livekit.IngressState_ENDPOINT_PUBLISHING}}
	if isIngressTerminal(info) {
		t.Fatal("publishing should not be terminal")
	}
	info.State.Status = livekit.IngressState_ENDPOINT_COMPLETE
	if !isIngressTerminal(info) {
		t.Fatal("complete should be terminal")
	}
}
