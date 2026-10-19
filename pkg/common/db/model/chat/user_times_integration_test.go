package chat

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	chattable "chat/pkg/common/db/table/chat"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Opt-in: CHAT_TEST_MONGO_URI=mongodb://user:pass@127.0.0.1:47017/?authSource=admin go test ./pkg/common/db/model/chat -run UserTimes
// Uses a throwaway database that is dropped afterwards.
func TestUserTimesIntegration(t *testing.T) {
	uri := os.Getenv("CHAT_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("CHAT_TEST_MONGO_URI not set")
	}
	ctx := context.Background()
	cli, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	db := cli.Database(fmt.Sprintf("tmp_user_times_%d", time.Now().UnixNano()))
	defer func() { _ = db.Drop(ctx); _ = cli.Disconnect(ctx) }()

	login, err := NewUserLoginRecord(db)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegister(db)
	if err != nil {
		t.Fatal(err)
	}
	t1 := time.UnixMilli(1760000000000)
	t2 := time.UnixMilli(1760000500000)
	if err := login.Create(ctx,
		&chattable.UserLoginRecord{UserID: "u1", LoginTime: t1},
		&chattable.UserLoginRecord{UserID: "u1", LoginTime: t2},
		&chattable.UserLoginRecord{UserID: "u2", LoginTime: t1}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Create(ctx, &chattable.Register{UserID: "u1", CreateTime: t1}); err != nil {
		t.Fatal(err)
	}
	got, err := login.FindLastLoginTimes(ctx, []string{"u1", "u2", "u3"})
	if err != nil {
		t.Fatal(err)
	}
	if got["u1"].UnixMilli() != t2.UnixMilli() || got["u2"].UnixMilli() != t1.UnixMilli() {
		t.Fatalf("last login: %v", got)
	}
	if _, ok := got["u3"]; ok {
		t.Fatal("u3 has no login record")
	}
	rt, err := reg.FindCreateTimes(ctx, []string{"u1", "u2"})
	if err != nil || rt["u1"].UnixMilli() != t1.UnixMilli() || len(rt) != 1 {
		t.Fatalf("register times %v %v", rt, err)
	}
	before := time.UnixMilli(1760000200000)
	n, err := login.CountTotal(ctx, &before)
	if err != nil || n != 2 {
		t.Fatalf("CountTotal(before) = %d %v, want 2", n, err)
	}
}
