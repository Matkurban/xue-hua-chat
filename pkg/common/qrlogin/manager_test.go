package qrlogin_test

import (
	"context"
	"testing"
	"time"

	"chat/pkg/common/qrlogin"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestManager(t *testing.T) (*qrlogin.Manager, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mgr := qrlogin.NewManager(rdb, 120*time.Second)
	return mgr, func() {
		_ = rdb.Close()
		mr.Close()
	}
}

func TestQrLoginSessionLifecycle(t *testing.T) {
	mgr, cleanup := newTestManager(t)
	defer cleanup()
	ctx := context.Background()

	_, sessionID, err := mgr.Create(ctx, 3, "device-1", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	session, err := mgr.Get(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != qrlogin.StatusPending {
		t.Fatalf("expected pending, got %s", session.Status)
	}

	session, err = mgr.Scan(ctx, sessionID, "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != qrlogin.StatusScanned {
		t.Fatalf("expected scanned, got %s", session.Status)
	}

	session, err = mgr.Confirm(ctx, sessionID, "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != qrlogin.StatusConfirmed || session.UserID != "user-a" {
		t.Fatalf("unexpected confirm result: %+v", session)
	}

	consumed, err := mgr.TryConsumeConfirmed(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if consumed.Status != qrlogin.StatusConsumed {
		t.Fatalf("expected consumed, got %s", consumed.Status)
	}

	_, err = mgr.TryConsumeConfirmed(ctx, sessionID)
	if err != qrlogin.ErrAlreadyConsumed {
		t.Fatalf("expected ErrAlreadyConsumed, got %v", err)
	}
}

func TestQrLoginRollbackConsumed(t *testing.T) {
	mgr, cleanup := newTestManager(t)
	defer cleanup()
	ctx := context.Background()

	_, sessionID, err := mgr.Create(ctx, 4, "device-2", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Confirm(ctx, sessionID, "user-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.TryConsumeConfirmed(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := mgr.RollbackConsumed(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	session, err := mgr.Get(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != qrlogin.StatusConfirmed {
		t.Fatalf("expected confirmed after rollback, got %s", session.Status)
	}
}

func TestQrLoginCancelResetsPending(t *testing.T) {
	mgr, cleanup := newTestManager(t)
	defer cleanup()
	ctx := context.Background()

	_, sessionID, err := mgr.Create(ctx, 7, "device-3", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Scan(ctx, sessionID, "user-c"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Cancel(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	session, err := mgr.Get(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != qrlogin.StatusPending || session.ScannedBy != "" {
		t.Fatalf("expected pending with empty scannedBy, got %+v", session)
	}
}

func TestTryConsumeConfirmedConcurrentOnlyOneSucceeds(t *testing.T) {
	mgr, cleanup := newTestManager(t)
	defer cleanup()
	ctx := context.Background()

	_, sessionID, err := mgr.Create(ctx, 3, "device-5", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Confirm(ctx, sessionID, "user-d"); err != nil {
		t.Fatal(err)
	}

	type result struct {
		session *qrlogin.Session
		err     error
	}
	results := make(chan result, 8)
	for i := 0; i < 8; i++ {
		go func() {
			s, err := mgr.TryConsumeConfirmed(ctx, sessionID)
			results <- result{s, err}
		}()
	}

	var success int
	var alreadyConsumed int
	for i := 0; i < 8; i++ {
		r := <-results
		switch {
		case r.err == nil:
			success++
			if r.session.Status != qrlogin.StatusConsumed {
				t.Fatalf("expected consumed, got %s", r.session.Status)
			}
		case r.err == qrlogin.ErrAlreadyConsumed:
			alreadyConsumed++
		default:
			t.Fatalf("unexpected error: %v", r.err)
		}
	}
	if success != 1 {
		t.Fatalf("expected exactly one successful consume, got %d", success)
	}
	if alreadyConsumed != 7 {
		t.Fatalf("expected 7 ErrAlreadyConsumed, got %d", alreadyConsumed)
	}
}

func TestQrLoginConfirmRejectsOtherUser(t *testing.T) {
	mgr, cleanup := newTestManager(t)
	defer cleanup()
	ctx := context.Background()

	_, sessionID, err := mgr.Create(ctx, 3, "device-4", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Scan(ctx, sessionID, "user-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Confirm(ctx, sessionID, "user-b"); err != qrlogin.ErrScannedByOther {
		t.Fatalf("expected ErrScannedByOther, got %v", err)
	}
}
