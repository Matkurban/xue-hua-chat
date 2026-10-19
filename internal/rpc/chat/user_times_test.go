package chat

import (
	"testing"
	"time"

	"chat/pkg/common/db/table/chat"
	"chat/pkg/protocol/common"
)

func TestDbToPbUserFullInfoCreateTime(t *testing.T) {
	ts := time.UnixMilli(1760000000123)
	got := DbToPbUserFullInfo(&chat.Attribute{UserID: "u1", CreateTime: ts})
	if got.CreateTime != 1760000000123 {
		t.Fatalf("createTime=%d", got.CreateTime)
	}
	if got := DbToPbUserFullInfo(&chat.Attribute{UserID: "u2"}); got.CreateTime != 0 {
		t.Fatalf("zero time must map to 0, got %d", got.CreateTime)
	}
}

func TestApplyUserTimes(t *testing.T) {
	users := []*common.UserFullInfo{
		{UserID: "a", CreateTime: 111}, // attribute value wins
		{UserID: "b"},                  // falls back to register table
		{UserID: "c"},                  // never logged in, unknown
		nil,
	}
	applyUserTimes(users,
		map[string]int64{"a": 999, "b": 222},
		map[string]int64{"a": 333, "b": 444})
	if users[0].CreateTime != 111 || users[0].LastLoginTime != 333 {
		t.Fatalf("a: %+v", users[0])
	}
	if users[1].CreateTime != 222 || users[1].LastLoginTime != 444 {
		t.Fatalf("b: %+v", users[1])
	}
	if users[2].CreateTime != 0 || users[2].LastLoginTime != 0 {
		t.Fatalf("c: %+v", users[2])
	}
}
