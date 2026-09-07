package chat

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeConferenceTitle(t *testing.T) {
	ok, err := NormalizeConferenceTitle("  周会  ")
	if err != nil || ok != "周会" {
		t.Fatalf("got %q %v", ok, err)
	}
	if _, err := NormalizeConferenceTitle("   "); err == nil {
		t.Fatal("expected empty rejected")
	}
	long := strings.Repeat("测", ConferenceTitleMaxRunes+1)
	if utf8.RuneCountInString(long) <= ConferenceTitleMaxRunes {
		t.Fatal("fixture too short")
	}
	if _, err := NormalizeConferenceTitle(long); err == nil {
		t.Fatal("expected too long rejected")
	}
	exact := strings.Repeat("a", ConferenceTitleMaxRunes)
	if got, err := NormalizeConferenceTitle(exact); err != nil || got != exact {
		t.Fatalf("exact max: %q %v", got, err)
	}
}

func TestCreateConferenceReq_Check_Title(t *testing.T) {
	req := &CreateConferenceReq{
		CreatorUserID: "u1", CallType: "video", Title: "  产品评审  ",
	}
	if err := req.Check(); err != nil {
		t.Fatal(err)
	}
	if req.Title != "产品评审" {
		t.Fatalf("title not normalized: %q", req.Title)
	}
	req.Title = ""
	if err := req.Check(); err == nil {
		t.Fatal("expected title required")
	}
}
