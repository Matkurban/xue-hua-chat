package rtc

import (
	"fmt"
	"regexp"
	"testing"
)

var conferenceRoomIDPattern = regexp.MustCompile(`^[0-9]{9}$`)

func TestGenerateConferenceRoomID_Format(t *testing.T) {
	for i := 0; i < 64; i++ {
		id, err := generateConferenceRoomID()
		if err != nil {
			t.Fatalf("generateConferenceRoomID: %v", err)
		}
		if len(id) != 9 {
			t.Fatalf("len=%d want 9, id=%q", len(id), id)
		}
		if !conferenceRoomIDPattern.MatchString(id) {
			t.Fatalf("id %q does not match ^[0-9]{9}$", id)
		}
	}
}

func TestConferenceRoomID_ZeroPaddingContract(t *testing.T) {
	// Same %09d contract as generateConferenceRoomID.
	padded := fmt.Sprintf("%09d", int64(42))
	if padded != "000000042" {
		t.Fatalf("got %q want 000000042", padded)
	}
	if !conferenceRoomIDPattern.MatchString(padded) {
		t.Fatalf("padded id %q invalid", padded)
	}
}
