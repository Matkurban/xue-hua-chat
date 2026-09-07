package rtc

import (
	"context"
	"testing"

	"chat/pkg/common/imapi"
	"github.com/openimsdk/protocol/sdkws"
)

type nicknameStubIM struct {
	imapi.CallerInterface
	nicks map[string]string
}

func (n *nicknameStubIM) ImAdminTokenWithDefaultAdmin(context.Context) (string, error) {
	return "tok", nil
}

func (n *nicknameStubIM) GetUserInfo(ctx context.Context, userID string) (*sdkws.UserInfo, error) {
	users, err := n.GetUsersInfo(ctx, []string{userID})
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	return users[0], nil
}

func (n *nicknameStubIM) GetUsersInfo(_ context.Context, userIDs []string) ([]*sdkws.UserInfo, error) {
	out := make([]*sdkws.UserInfo, 0, len(userIDs))
	for _, id := range userIDs {
		out = append(out, &sdkws.UserInfo{
			UserID:   id,
			Nickname: n.nicks[id],
			FaceURL:  "face/" + id,
		})
	}
	return out, nil
}

func (n *nicknameStubIM) SendBusinessNotification(context.Context, string, string, string, string) error {
	return nil
}

func TestListParticipantsKeywordMatchesEnrichedNickname(t *testing.T) {
	store, _, cleanup := newTestConferenceStore(t)
	defer cleanup()

	im := &nicknameStubIM{nicks: map[string]string{
		"admin": "房主甲",
		"u1":    "张三",
		"u2":    "李四",
	}}
	m := &ConferenceManager{
		store:  store,
		im:     im,
		stopCh: make(chan struct{}),
	}

	roomID := "100000201"
	seedRoomWithMembers(t, m.store, roomID, "admin", []*ConferenceMember{
		{UserID: "admin", Role: ConferenceRoleAdmin, JoinTime: 1},
		{UserID: "u1", Role: ConferenceRoleUser, JoinTime: 2},
		{UserID: "u2", Role: ConferenceRoleUser, JoinTime: 3},
	})

	page, err := m.ListParticipants(roomID, 1, 20, "", "张")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.List) != 1 || page.List[0].UserID != "u1" {
		t.Fatalf("expected only 张三, got total=%d list=%+v", page.Total, page.List)
	}
	if page.List[0].Nickname != "张三" {
		t.Fatalf("nickname not enriched: %+v", page.List[0])
	}

	// Persisted for subsequent searches without re-fetch gaps.
	mem, ok := m.GetMember(roomID, "u1")
	if !ok || mem.Nickname != "张三" {
		t.Fatalf("expected nickname persisted, got %+v ok=%v", mem, ok)
	}
}

func TestFillMemberProfileOnCreate(t *testing.T) {
	m, _, cleanup := newTestConferenceManager(t)
	defer cleanup()
	m.im = &nicknameStubIM{nicks: map[string]string{"admin": "主持人"}}

	room, _, err := m.CreateConference("admin", "video", "t", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mem, ok := m.GetMember(room.RoomID, "admin")
	if !ok || mem.Nickname != "主持人" {
		t.Fatalf("creator nickname missing: %+v ok=%v", mem, ok)
	}
}
