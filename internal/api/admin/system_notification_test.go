package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"chat/pkg/common/db/table/admin"
	"chat/pkg/common/imapi"
	"gitee.com/matkurban/openimsdk-tools/db/pagination"
	"github.com/gin-gonic/gin"
)

func TestPushDescTruncatesRunes(t *testing.T) {
	got := pushDesc("你好世界额外", 4)
	if got != "你好世界…" {
		t.Fatalf("got %q", got)
	}
	if pushDesc("short", 80) != "short" {
		t.Fatal("short text should stay intact")
	}
}

func TestInferImageType(t *testing.T) {
	if inferImageType("https://x/a.JPG?x=1") != "image/jpeg" {
		t.Fatal("jpeg")
	}
	if inferImageType("https://x/a.webp") != "image/webp" {
		t.Fatal("webp")
	}
	if inferImageType("https://x/a.bin") != "image/png" {
		t.Fatal("default png")
	}
}

func TestPendingRevokeItemsSkipsAlreadyRevoked(t *testing.T) {
	items, ids := pendingRevokeItems([]*admin.SystemNotificationTarget{
		{RecvID: "u1", ClientMsgID: "c1", Status: admin.SystemNotificationTargetSent},
		{RecvID: "u2", ClientMsgID: "c2", Status: admin.SystemNotificationTargetRevoked},
		nil,
		{RecvID: "u3", ClientMsgID: "c3", Status: admin.SystemNotificationTargetFailed},
	})
	if len(items) != 2 || len(ids) != 2 {
		t.Fatalf("pending=%d ids=%d", len(items), len(ids))
	}
	if items[0].RecvID != "u1" || items[1].RecvID != "u3" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

type fakeNotifStore struct {
	mu             sync.Mutex
	n              *admin.SystemNotification
	createFailLeft int
	createErr      error
	createCalls    int
}

func (f *fakeNotifStore) Create(_ context.Context, n *admin.SystemNotification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if f.createFailLeft > 0 {
		f.createFailLeft--
		if f.createErr != nil {
			return f.createErr
		}
	}
	cp := *n
	f.n = &cp
	return nil
}

func (f *fakeNotifStore) Take(_ context.Context, id string) (*admin.SystemNotification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.n == nil || f.n.NotificationID != id {
		return nil, errRecord("not found")
	}
	cp := *f.n
	return &cp, nil
}

func (f *fakeNotifStore) Update(_ context.Context, id string, update map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.n == nil || f.n.NotificationID != id {
		return errRecord("not found")
	}
	if v, ok := update["status"].(string); ok {
		f.n.Status = v
	}
	if v, ok := update["revoke_by"].(string); ok {
		f.n.RevokeBy = v
	}
	if v, ok := update["revoke_success"].(int32); ok {
		f.n.RevokeSuccess = v
	}
	if v, ok := update["revoke_failed"].(int32); ok {
		f.n.RevokeFailed = v
	}
	if v, ok := update["revoke_time"].(time.Time); ok {
		f.n.RevokeTime = v
	}
	return nil
}

func (f *fakeNotifStore) Search(context.Context, pagination.Pagination) (int64, []*admin.SystemNotification, error) {
	return 0, nil, nil
}

type fakeTargets struct {
	mu                 sync.Mutex
	items              []*admin.SystemNotificationTarget
	createManyFailLeft int
	createManyErr      error
	createManyCalls    int
}

func (f *fakeTargets) CreateMany(_ context.Context, items []*admin.SystemNotificationTarget) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createManyCalls++
	if f.createManyFailLeft > 0 {
		f.createManyFailLeft--
		if f.createManyErr != nil {
			return f.createManyErr
		}
	}
	f.items = append(f.items, items...)
	return nil
}

func (f *fakeTargets) ListByNotification(_ context.Context, notificationID string, skip, limit int64) ([]*admin.SystemNotificationTarget, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []*admin.SystemNotificationTarget
	for _, it := range f.items {
		if it != nil && it.NotificationID == notificationID {
			cp := *it
			all = append(all, &cp)
		}
	}
	if skip >= int64(len(all)) {
		return nil, nil
	}
	end := skip + limit
	if end > int64(len(all)) {
		end = int64(len(all))
	}
	return all[skip:end], nil
}

func (f *fakeTargets) CountByNotification(_ context.Context, notificationID string, status string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, it := range f.items {
		if it == nil || it.NotificationID != notificationID {
			continue
		}
		if status == "" || it.Status == status {
			n++
		}
	}
	return n, nil
}

func (f *fakeTargets) MarkStatus(_ context.Context, notificationID string, recvIDs []string, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := map[string]struct{}{}
	for _, id := range recvIDs {
		want[id] = struct{}{}
	}
	for _, it := range f.items {
		if it != nil && it.NotificationID == notificationID {
			if _, ok := want[it.RecvID]; ok {
				it.Status = status
			}
		}
	}
	return nil
}

type revokeStubIM struct {
	imapi.CallerInterface
	mu          sync.Mutex
	revokeCalls int
	lastItems   []imapi.BatchRevokeMsgItem
}

func (s *revokeStubIM) ImAdminTokenWithDefaultAdmin(context.Context) (string, error) {
	return "tok", nil
}

func (s *revokeStubIM) BatchRevokeMsg(_ context.Context, req *imapi.BatchRevokeMsgReq) (*imapi.BatchRevokeMsgResp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokeCalls++
	if req != nil {
		s.lastItems = append([]imapi.BatchRevokeMsgItem(nil), req.Items...)
	}
	return &imapi.BatchRevokeMsgResp{SuccessCount: len(req.Items)}, nil
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func errRecord(msg string) error { return simpleError(msg) }

func callRevoke(api *SystemNotificationApi, notificationID string) (errCode float64, data map[string]any) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, _ := json.Marshal(map[string]string{"notificationID": notificationID})
	c.Request = httptest.NewRequest(http.MethodPost, "/system_notification/revoke", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	api.Revoke(c)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if v, ok := resp["errCode"].(float64); ok {
		errCode = v
	}
	if d, ok := resp["data"].(map[string]any); ok {
		data = d
	}
	return errCode, data
}

func TestRevokeAlreadyRevokedDoesNotCallIM(t *testing.T) {
	store := &fakeNotifStore{n: &admin.SystemNotification{
		NotificationID: "n1",
		SendID:         "imAdmin",
		SuccessCount:   2,
		Status:         admin.SystemNotificationStatusRevoked,
	}}
	im := &revokeStubIM{}
	api := NewSystemNotificationApi(store, &fakeTargets{}, im, "imAdmin")
	code, data := callRevoke(api, "n1")
	if code != 0 {
		t.Fatalf("errCode=%v data=%v", code, data)
	}
	if im.revokeCalls != 0 {
		t.Fatalf("duplicate revoke should not call IM, calls=%d", im.revokeCalls)
	}
}

func TestRevokeSpecifiedUsersSkipsRevokedTargets(t *testing.T) {
	store := &fakeNotifStore{n: &admin.SystemNotification{
		NotificationID: "n2",
		SendID:         "imAdmin",
		SuccessCount:   2,
		Status:         admin.SystemNotificationStatusSent,
		IsSendAll:      false,
	}}
	targets := &fakeTargets{items: []*admin.SystemNotificationTarget{
		{NotificationID: "n2", RecvID: "u1", ClientMsgID: "c1", Status: admin.SystemNotificationTargetSent},
		{NotificationID: "n2", RecvID: "u2", ClientMsgID: "c2", Status: admin.SystemNotificationTargetRevoked},
	}}
	im := &revokeStubIM{}
	api := NewSystemNotificationApi(store, targets, im, "imAdmin")
	code, data := callRevoke(api, "n2")
	if code != 0 {
		t.Fatalf("errCode=%v data=%v", code, data)
	}
	if im.revokeCalls != 1 || len(im.lastItems) != 1 || im.lastItems[0].RecvID != "u1" {
		t.Fatalf("calls=%d items=%+v", im.revokeCalls, im.lastItems)
	}
	if data["status"] != admin.SystemNotificationStatusRevoked {
		t.Fatalf("status=%v", data["status"])
	}
}

func TestRevokeAllUsersTargets(t *testing.T) {
	store := &fakeNotifStore{n: &admin.SystemNotification{
		NotificationID: "n3",
		SendID:         "imAdmin",
		SuccessCount:   3,
		Status:         admin.SystemNotificationStatusSent,
		IsSendAll:      true,
	}}
	targets := &fakeTargets{items: []*admin.SystemNotificationTarget{
		{NotificationID: "n3", RecvID: "a", ClientMsgID: "c", Status: admin.SystemNotificationTargetSent},
		{NotificationID: "n3", RecvID: "b", ClientMsgID: "c", Status: admin.SystemNotificationTargetSent},
		{NotificationID: "n3", RecvID: "c", ClientMsgID: "c", Status: admin.SystemNotificationTargetSent},
	}}
	im := &revokeStubIM{}
	api := NewSystemNotificationApi(store, targets, im, "imAdmin")
	code, data := callRevoke(api, "n3")
	if code != 0 {
		t.Fatalf("errCode=%v data=%v", code, data)
	}
	if im.revokeCalls != 1 || len(im.lastItems) != 3 {
		t.Fatalf("calls=%d items=%d", im.revokeCalls, len(im.lastItems))
	}
	if data["status"] != admin.SystemNotificationStatusRevoked {
		t.Fatalf("status=%v", data["status"])
	}
}

func TestPersistWithRetryEventuallySucceeds(t *testing.T) {
	n := 0
	err := persistWithRetry(context.Background(), func() error {
		n++
		if n < persistRetryAttempts {
			return errRecord("tmp")
		}
		return nil
	})
	if err != nil || n != persistRetryAttempts {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

func TestPersistWithRetryGivesUp(t *testing.T) {
	n := 0
	err := persistWithRetry(context.Background(), func() error {
		n++
		return errRecord("tmp")
	})
	if err == nil || n != persistRetryAttempts {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

type sendStubIM struct {
	imapi.CallerInterface
}

func (s *sendStubIM) ImAdminTokenWithDefaultAdmin(context.Context) (string, error) {
	return "tok", nil
}

func (s *sendStubIM) BatchSendMsg(_ context.Context, _ *imapi.BatchSendMsgReq) (*imapi.BatchSendMsgResp, error) {
	return &imapi.BatchSendMsgResp{Results: []*imapi.BatchSendMsgResult{
		{RecvID: "u1", ClientMsgID: "c1", ServerMsgID: "s1"},
	}}, nil
}

func callSend(api *SystemNotificationApi) (errCode float64, errText string, data map[string]any) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, _ := json.Marshal(map[string]any{
		"notificationType": 1,
		"notificationName": "title",
		"text":             "body",
		"isSendAll":        false,
		"recvIDs":          []string{"u1"},
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/system_notification/send", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	api.Send(c)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if v, ok := resp["errCode"].(float64); ok {
		errCode = v
	}
	if s, ok := resp["errMsg"].(string); ok {
		errText = s
	}
	if s, ok := resp["errDlt"].(string); ok && s != "" {
		errText = s
	}
	if d, ok := resp["data"].(map[string]any); ok {
		data = d
	}
	return errCode, errText, data
}

func TestSendReturnsErrorWhenCreateManyFails(t *testing.T) {
	store := &fakeNotifStore{}
	targets := &fakeTargets{createManyFailLeft: persistRetryAttempts, createManyErr: errRecord("mongo down")}
	api := NewSystemNotificationApi(store, targets, &sendStubIM{}, "imAdmin")
	code, msg, _ := callSend(api)
	if code == 0 {
		t.Fatal("CreateMany failure must not look like publish success")
	}
	if !strings.Contains(msg, "recipients") {
		t.Fatalf("want recipients persist error, got %q", msg)
	}
	if targets.createManyCalls != persistRetryAttempts {
		t.Fatalf("retries=%d want %d", targets.createManyCalls, persistRetryAttempts)
	}
}

func TestSendRetriesCreateManyThenSucceeds(t *testing.T) {
	store := &fakeNotifStore{}
	targets := &fakeTargets{createManyFailLeft: 2, createManyErr: errRecord("temp")}
	api := NewSystemNotificationApi(store, targets, &sendStubIM{}, "imAdmin")
	code, _, data := callSend(api)
	if code != 0 {
		t.Fatalf("errCode=%v", code)
	}
	if data["resultsCount"] != float64(1) {
		t.Fatalf("resultsCount=%v", data["resultsCount"])
	}
	if len(targets.items) != 1 {
		t.Fatalf("targets=%d", len(targets.items))
	}
}

func TestSendReturnsErrorWhenCreateFails(t *testing.T) {
	store := &fakeNotifStore{createFailLeft: persistRetryAttempts, createErr: errRecord("mongo down")}
	api := NewSystemNotificationApi(store, &fakeTargets{}, &sendStubIM{}, "imAdmin")
	code, msg, _ := callSend(api)
	if code == 0 {
		t.Fatal("Create failure must not look like publish success")
	}
	if !strings.Contains(msg, "send record") {
		t.Fatalf("want send-record persist error, got %q", msg)
	}
}
