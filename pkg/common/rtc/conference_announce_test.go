package rtc

import "testing"

func TestParseConversationTarget(t *testing.T) {
	t.Run("group", func(t *testing.T) {
		st, recv, gid := parseConversationTarget("sg_g1", "", "host1")
		if st != groupChatSessionType || recv != "" || gid != "g1" {
			t.Fatalf("got session=%d recv=%q gid=%q", st, recv, gid)
		}
	})
	t.Run("singleHostIsA", func(t *testing.T) {
		st, recv, gid := parseConversationTarget("si_host1_peer2", "", "host1")
		if st != singleChatSessionType || recv != "peer2" || gid != "" {
			t.Fatalf("got session=%d recv=%q gid=%q", st, recv, gid)
		}
	})
	t.Run("singleHostIsB", func(t *testing.T) {
		st, recv, gid := parseConversationTarget("si_peer2_host1", "", "host1")
		if st != singleChatSessionType || recv != "peer2" || gid != "" {
			t.Fatalf("got session=%d recv=%q gid=%q", st, recv, gid)
		}
	})
	t.Run("fallbackGroup", func(t *testing.T) {
		st, recv, gid := parseConversationTarget("unknown", "g9", "host1")
		if st != groupChatSessionType || recv != "" || gid != "g9" {
			t.Fatalf("got session=%d recv=%q gid=%q", st, recv, gid)
		}
	})
	t.Run("unresolvable", func(t *testing.T) {
		st, recv, gid := parseConversationTarget("", "", "host1")
		if st != 0 || recv != "" || gid != "" {
			t.Fatalf("got session=%d recv=%q gid=%q", st, recv, gid)
		}
	})
}
