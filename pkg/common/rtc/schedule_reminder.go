package rtc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"gitee.com/matkurban/openimsdk-tools/log"
	"github.com/redis/go-redis/v9"
)

const (
	ContentTypeMeetingReminder = 128

	scheduleRemindersKey     = "schedule:reminders"
	scheduleReminderDonePref = "schedule:reminder:done:"
)

// ReminderTaskStore Redis ZSET 会前提醒
type ReminderTaskStore struct {
	rdb redis.UniversalClient
}

func NewReminderTaskStore(rdb redis.UniversalClient) *ReminderTaskStore {
	return &ReminderTaskStore{rdb: rdb}
}

func reminderMember(occurrenceID string, minutes int) string {
	return fmt.Sprintf("%s:%d", occurrenceID, minutes)
}

func parseReminderMember(member string) (occurrenceID string, minutes int, ok bool) {
	i := strings.LastIndex(member, ":")
	if i <= 0 {
		return "", 0, false
	}
	occurrenceID = member[:i]
	m, err := strconv.Atoi(member[i+1:])
	if err != nil {
		return "", 0, false
	}
	return occurrenceID, m, true
}

// ScheduleReminders 为场次注册提醒。
func (s *ReminderTaskStore) ScheduleReminders(ctx context.Context, occurrenceID string, startTime int64, remindMinutes []int) error {
	if s == nil || s.rdb == nil || occurrenceID == "" {
		return nil
	}
	now := time.Now().UnixMilli()
	pipe := s.rdb.Pipeline()
	for _, m := range remindMinutes {
		if m <= 0 {
			continue
		}
		fireAt := startTime - int64(m)*60*1000
		if fireAt < now {
			continue
		}
		pipe.ZAdd(ctx, scheduleRemindersKey, redis.Z{
			Score:  float64(fireAt),
			Member: reminderMember(occurrenceID, m),
		})
	}
	_, err := pipe.Exec(ctx)
	return err
}

// RemoveOccurrenceReminders 删除某场次全部提醒。
func (s *ReminderTaskStore) RemoveOccurrenceReminders(ctx context.Context, occurrenceID string, remindMinutes []int) {
	if s == nil || s.rdb == nil {
		return
	}
	members := make([]any, 0, len(remindMinutes))
	for _, m := range remindMinutes {
		members = append(members, reminderMember(occurrenceID, m))
	}
	if len(members) == 0 {
		// fallback: scan matching prefix via ZRANGE — keep simple: known minutes set
		for _, m := range []int{5, 10, 15, 20, 25, 30} {
			members = append(members, reminderMember(occurrenceID, m))
		}
	}
	_ = s.rdb.ZRem(ctx, scheduleRemindersKey, members...).Err()
}

// ClaimDue 拉取并抢占到期提醒。
func (s *ReminderTaskStore) ClaimDue(ctx context.Context, nowMs int64, limit int64) ([]string, error) {
	if s == nil || s.rdb == nil {
		return nil, nil
	}
	members, err := s.rdb.ZRangeByScore(ctx, scheduleRemindersKey, &redis.ZRangeBy{
		Min: "-inf", Max: strconv.FormatInt(nowMs, 10), Offset: 0, Count: limit,
	}).Result()
	if err != nil {
		return nil, err
	}
	claimed := make([]string, 0, len(members))
	for _, m := range members {
		n, err := s.rdb.ZRem(ctx, scheduleRemindersKey, m).Result()
		if err != nil || n == 0 {
			continue
		}
		claimed = append(claimed, m)
	}
	return claimed, nil
}

// MarkDone 防重。返回 (claimed, redisOK)：
// - claimed=false 表示已被其它实例处理；
// - redisOK=false 表示 Redis 出错，调用方应 requeue 而非丢弃。
func (s *ReminderTaskStore) MarkDone(ctx context.Context, occurrenceID string, minutes int) (claimed bool, redisOK bool) {
	if s == nil || s.rdb == nil {
		return true, true
	}
	ok, err := s.rdb.SetNX(ctx, scheduleReminderDonePref+reminderMember(occurrenceID, minutes), "1", 48*time.Hour).Result()
	if err != nil {
		return false, false
	}
	return ok, true
}

// ClearDone 清除防重键以便重试。
func (s *ReminderTaskStore) ClearDone(ctx context.Context, occurrenceID string, minutes int) {
	if s == nil || s.rdb == nil {
		return
	}
	_ = s.rdb.Del(ctx, scheduleReminderDonePref+reminderMember(occurrenceID, minutes)).Err()
}

// Requeue 发送失败后延迟重入队。
func (s *ReminderTaskStore) Requeue(ctx context.Context, member string, delay time.Duration) {
	if s == nil || s.rdb == nil {
		return
	}
	_ = s.rdb.ZAdd(ctx, scheduleRemindersKey, redis.Z{
		Score:  float64(time.Now().Add(delay).UnixMilli()),
		Member: member,
	}).Err()
}

// ReminderWorker 扫描并发送 ContentType 128。
type ReminderWorker struct {
	store       *ReminderTaskStore
	loadFn      func(ctx context.Context, occurrenceID string) (meetingID, title, callType string, startTime int64, recipients []string, ok bool)
	im          imapi.CallerInterface
	adminUserID string
	stopCh      chan struct{}
}

func NewReminderWorker(
	store *ReminderTaskStore,
	im imapi.CallerInterface,
	adminUserID string,
	loadFn func(ctx context.Context, occurrenceID string) (meetingID, title, callType string, startTime int64, recipients []string, ok bool),
) *ReminderWorker {
	return &ReminderWorker{
		store: store, im: im, adminUserID: adminUserID, loadFn: loadFn, stopCh: make(chan struct{}),
	}
}

func (w *ReminderWorker) Start(ctx context.Context) {
	go w.loop(ctx)
}

func (w *ReminderWorker) Stop() {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}

func (w *ReminderWorker) loop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *ReminderWorker) tick(ctx context.Context) {
	if w == nil || w.store == nil || w.loadFn == nil || w.im == nil {
		return
	}
	members, err := w.store.ClaimDue(ctx, time.Now().UnixMilli(), 100)
	if err != nil {
		log.ZWarn(ctx, "reminder claim failed", err)
		return
	}
	for _, member := range members {
		occID, minutes, ok := parseReminderMember(member)
		if !ok {
			continue
		}
		claimed, redisOK := w.store.MarkDone(ctx, occID, minutes)
		if !redisOK {
			w.store.Requeue(ctx, member, 5*time.Second)
			continue
		}
		if !claimed {
			continue
		}
		meetingID, title, callType, startTime, recipients, ok := w.loadFn(ctx, occID)
		if !ok || len(recipients) == 0 {
			// Transient load failure or cancelled occ: requeue briefly if load looks transient.
			if !ok {
				w.store.ClearDone(ctx, occID, minutes)
				w.store.Requeue(ctx, member, 30*time.Second)
			}
			continue
		}
		if err := w.sendReminders(ctx, meetingID, occID, title, callType, startTime, minutes, recipients); err != nil {
			log.ZWarn(ctx, "send meeting reminder failed", err, "occurrenceID", occID)
			w.store.ClearDone(ctx, occID, minutes)
			w.store.Requeue(ctx, member, 30*time.Second)
		}
	}
}

func (w *ReminderWorker) sendReminders(
	ctx context.Context, meetingID, occID, title, callType string, startTime int64, minutes int, recipients []string,
) error {
	tok, err := w.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		return err
	}
	apiCtx := mctx.WithApiToken(ctx, tok)
	var firstErr error
	for _, uid := range recipients {
		req := &imapi.SendCustomMsgReq{
			SendID:           w.adminUserID,
			RecvID:           uid,
			SessionType:      1,
			ContentType:      ContentTypeMeetingReminder,
			SenderPlatformID: 5,
			Content: map[string]any{
				"meetingID":     meetingID,
				"occurrenceID":  occID,
				"title":         title,
				"startTime":     startTime,
				"remindMinutes": int32(minutes),
				"callType":      callType,
			},
		}
		if err := w.im.SendCustomMessage(apiCtx, req); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
