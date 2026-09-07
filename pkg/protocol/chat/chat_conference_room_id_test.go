package chat

import "testing"

func TestCheckConferenceRoomID(t *testing.T) {
	cases := []struct {
		id   string
		ok   bool
		name string
	}{
		{name: "valid", id: "123456789", ok: true},
		{name: "leadingZeros", id: "000000042", ok: true},
		{name: "empty", id: "", ok: false},
		{name: "short", id: "12345678", ok: false},
		{name: "long", id: "1234567890", ok: false},
		{name: "uuid", id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890", ok: false},
		{name: "letters", id: "12345678a", ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckConferenceRoomID(tc.id)
			if tc.ok && err != nil {
				t.Fatalf("CheckConferenceRoomID(%q)=%v, want nil", tc.id, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("CheckConferenceRoomID(%q)=nil, want error", tc.id)
			}
		})
	}
}

func TestJoinConferenceReq_Check_RoomID(t *testing.T) {
	if err := (&JoinConferenceReq{RoomID: "123456789", UserID: "u1"}).Check(); err != nil {
		t.Fatalf("valid join: %v", err)
	}
	if err := (&JoinConferenceReq{RoomID: "not-a-uuid-ok", UserID: "u1"}).Check(); err == nil {
		t.Fatal("expected invalid roomID rejected")
	}
	if err := (&JoinConferenceReq{RoomID: "123456789", UserID: ""}).Check(); err == nil {
		t.Fatal("expected empty userID rejected")
	}
}
