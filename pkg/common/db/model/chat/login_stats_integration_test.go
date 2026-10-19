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

// Opt-in (see user_times_integration_test.go). Verifies per-day distinct users bucketed in Asia/Shanghai.
func TestLoginStatsIntegration(t *testing.T) {
	uri := os.Getenv("CHAT_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("CHAT_TEST_MONGO_URI not set")
	}
	ctx := context.Background()
	cli, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	db := cli.Database(fmt.Sprintf("tmp_login_stats_%d", time.Now().UnixNano()))
	defer func() { _ = db.Drop(ctx); _ = cli.Disconnect(ctx) }()
	login, err := NewUserLoginRecord(db)
	if err != nil {
		t.Fatal(err)
	}
	sh := time.FixedZone("sh", 8*3600)
	at := func(d, h int) time.Time { return time.Date(2026, 10, d, h, 30, 0, 0, sh) }
	// 10-15 23:30 +08 is 15:30 UTC (same UTC day); 10-16 00:30 +08 is 10-15 16:30 UTC (UTC day differs!)
	recs := []*chattable.UserLoginRecord{
		{UserID: "a", LoginTime: at(15, 23)},
		{UserID: "a", LoginTime: at(15, 10)}, // same user same day -> counted once
		{UserID: "b", LoginTime: at(15, 9)},
		{UserID: "a", LoginTime: at(16, 0)}, // local day 16 (UTC day 15)
		{UserID: "c", LoginTime: at(17, 8)},
		{UserID: "z", LoginTime: at(20, 8)}, // outside range
	}
	if err := login.Create(ctx, recs...); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 15, 0, 0, 0, 0, sh)
	end := time.Date(2026, 10, 20, 0, 0, 0, 0, sh)
	m, distinct, err := login.CountRangeEverydayTotal(ctx, &start, &end)
	if err != nil {
		t.Fatal(err)
	}
	if m["2026-10-15"] != 2 || m["2026-10-16"] != 1 || m["2026-10-17"] != 1 || len(m) != 3 {
		t.Fatalf("bad per-day map: %v", m)
	}
	if distinct != 3 {
		t.Fatalf("distinct users want 3 got %d", distinct)
	}
}
