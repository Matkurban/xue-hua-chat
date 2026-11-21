package rtc

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"chat/pkg/common/db/database"
	chatdb "chat/pkg/common/db/table/chat"
	"chat/pkg/common/imapi"
	"chat/pkg/common/mctx"
	"chat/pkg/common/errs"
	"chat/pkg/common/log"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func normalizeScheduleTitle(title string) (string, error) {
	t := strings.TrimSpace(title)
	if t == "" {
		return "", errs.ErrArgs.WrapMsg("title is required")
	}
	if utf8.RuneCountInString(t) > scheduleTitleMaxRunes {
		return "", errs.ErrArgs.WrapMsg("title must be at most 40 characters")
	}
	return t, nil
}

const scheduleTitleMaxRunes = 40

var allowedRemindMinutes = map[int]struct{}{5: {}, 10: {}, 15: {}, 20: {}, 25: {}, 30: {}}

// ScheduleInvitees 邀请入参
type ScheduleInvitees struct {
	UserIDs         []string
	GroupIDs        []string
	ConversationIDs []string
}

// CreateScheduleInput 创建预约入参
type CreateScheduleInput struct {
	UserID                 string
	Title                  string
	CallType               string
	StartTime              int64
	DurationMinutes        int
	Timezone               string
	EnableWaitingRoom      bool
	MuteOnJoin             *bool
	AllowJoinBeforeMinutes *int
	RemindBeforeMinutes    []int
	Invitees               ScheduleInvitees
	Recurrence             chatdb.ScheduleRecurrence
	Password               string
}

// UpdateScheduleInput 修改预约
type UpdateScheduleInput struct {
	UserID                 string
	OccurrenceID           string
	Scope                  string // this|this_and_following|all
	Title                  *string
	StartTime              *int64
	DurationMinutes        *int
	Timezone               *string
	EnableWaitingRoom      *bool
	MuteOnJoin             *bool
	AllowJoinBeforeMinutes *int
	RemindBeforeMinutes    []int
	Invitees               *ScheduleInvitees
	Password               *string
	ClearPassword          bool
	Recurrence             *chatdb.ScheduleRecurrence
}

// ScheduleService 预约会议业务
type ScheduleService struct {
	db          database.ScheduleMeetingDatabaseInterface
	conf        *ConferenceManager
	im          imapi.CallerInterface
	adminUserID string
	rdb         redis.UniversalClient
	reminders   *ReminderTaskStore
	notifier    *ScheduleNotifier
	waiting     *WaitingRoomStore
	pwdLimiter  *PasswordRateLimiter
	worker      *ReminderWorker
}

func NewScheduleService(
	db database.ScheduleMeetingDatabaseInterface,
	conf *ConferenceManager,
	im imapi.CallerInterface,
	adminUserID string,
	rdb redis.UniversalClient,
) *ScheduleService {
	s := &ScheduleService{
		db: db, conf: conf, im: im, adminUserID: adminUserID, rdb: rdb,
		reminders:  NewReminderTaskStore(rdb),
		notifier:   NewScheduleNotifier(im, adminUserID),
		waiting:    NewWaitingRoomStore(rdb, im, adminUserID),
		pwdLimiter: NewPasswordRateLimiter(rdb),
	}
	if conf != nil {
		conf.SetWaitingRoomStore(s.waiting)
		conf.SetPasswordLimiter(s.pwdLimiter)
		conf.SetOnRoomEnded(func(roomID, occurrenceID string) {
			ctx := context.Background()
			if occurrenceID == "" {
				return
			}
			occ, err := s.db.GetOccurrence(ctx, occurrenceID)
			if err != nil || occ == nil {
				return
			}
			if occ.Status == chatdb.ScheduleOccurrenceStatusEnded || occ.Status == chatdb.ScheduleOccurrenceStatusCancelled {
				return
			}
			now := time.Now().UnixMilli()
			if now < occ.StartTime {
				occ.Status = chatdb.ScheduleOccurrenceStatusScheduled
				occ.ActivatedRoom = false
				occ.EndedAt = 0
				occ.EndReason = ""
			} else {
				occ.Status = chatdb.ScheduleOccurrenceStatusEnded
				occ.EndedAt = now
				occ.EndReason = chatdb.ScheduleEndReasonRoomEnded
			}
			_ = s.db.UpdateOccurrence(ctx, occ)
		})
	}
	s.worker = NewReminderWorker(s.reminders, im, adminUserID, s.loadReminderTarget)
	return s
}

func (s *ScheduleService) StartWorkers(ctx context.Context) {
	if s.worker != nil {
		s.worker.Start(ctx)
	}
}

func (s *ScheduleService) StopWorkers() {
	if s.worker != nil {
		s.worker.Stop()
	}
}

func (s *ScheduleService) Waiting() *WaitingRoomStore { return s.waiting }

func (s *ScheduleService) loadReminderTarget(ctx context.Context, occurrenceID string) (meetingID, title, callType string, startTime int64, recipients []string, ok bool) {
	occ, err := s.db.GetOccurrence(ctx, occurrenceID)
	if err != nil || occ == nil {
		return "", "", "", 0, nil, false
	}
	if occ.Status != chatdb.ScheduleOccurrenceStatusScheduled && occ.Status != chatdb.ScheduleOccurrenceStatusOngoing {
		return "", "", "", 0, nil, false
	}
	series, err := s.db.GetSeries(ctx, occ.SeriesID)
	if err != nil || series == nil {
		return "", "", "", 0, nil, false
	}
	title = effectiveTitle(series, occ)
	recipients = append([]string{series.HostUserID}, effectiveInvitees(series, occ)...)
	return series.MeetingID, title, series.CallType, occ.StartTime, uniqueStrings(recipients), true
}

func (s *ScheduleService) allocateMeetingID(ctx context.Context) (string, error) {
	for i := 0; i < conferenceRoomIDMaxAttempts; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(conferenceRoomIDMod))
		if err != nil {
			return "", err
		}
		id := fmt.Sprintf("%09d", n.Int64())
		exists, err := s.db.ExistsMeetingID(ctx, id)
		if err != nil {
			return "", err
		}
		if exists {
			continue
		}
		if s.conf != nil && s.conf.RoomExists(id) {
			continue
		}
		return id, nil
	}
	return "", fmt.Errorf("failed to allocate meeting ID")
}

// Create 创建预约
func (s *ScheduleService) Create(ctx context.Context, in CreateScheduleInput) (*chatdb.ScheduleMeetingSeries, *chatdb.ScheduleMeetingOccurrence, error) {
	if in.UserID == "" {
		return nil, nil, errs.ErrArgs.WrapMsg("userID is empty")
	}
	title, err := normalizeScheduleTitle(in.Title)
	if err != nil {
		return nil, nil, err
	}
	if in.CallType != "audio" && in.CallType != "video" {
		return nil, nil, errs.ErrArgs.WrapMsg("callType must be audio or video")
	}
	if in.StartTime < time.Now().UnixMilli()-60*1000 {
		return nil, nil, errs.ErrArgs.WrapMsg("startTime must not be in the past")
	}
	dur := in.DurationMinutes
	if dur == 0 {
		dur = 60
	}
	if dur < 15 || dur > 480 || dur%15 != 0 {
		return nil, nil, errs.ErrArgs.WrapMsg("durationMinutes must be 15-480 step 15")
	}
	if in.Timezone == "" {
		return nil, nil, errs.ErrArgs.WrapMsg("timezone is required")
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return nil, nil, errs.ErrArgs.WrapMsg("invalid timezone")
	}
	if err := ValidateMeetingPasswordFormat(in.Password); err != nil {
		return nil, nil, errs.ErrArgs.WrapMsg(err.Error())
	}
	hash, err := HashMeetingPassword(in.Password)
	if err != nil {
		return nil, nil, errs.ErrInternalServer.WrapMsg(err.Error())
	}
	muteOnJoin := true
	if in.MuteOnJoin != nil {
		muteOnJoin = *in.MuteOnJoin
	}
	allowBefore := 15
	if in.AllowJoinBeforeMinutes != nil {
		allowBefore = *in.AllowJoinBeforeMinutes
	}
	if allowBefore < 0 {
		allowBefore = 0
	}
	reminds := normalizeReminds(in.RemindBeforeMinutes)
	inviteeIDs, groupIDs, convIDs, err := s.resolveInvitees(ctx, in.UserID, in.Invitees)
	if err != nil {
		return nil, nil, err
	}
	starts, err := ExpandOccurrenceStarts(in.StartTime, in.Timezone, in.Recurrence)
	if err != nil {
		return nil, nil, errs.ErrArgs.WrapMsg(err.Error())
	}
	meetingID, err := s.allocateMeetingID(ctx)
	if err != nil {
		return nil, nil, errs.ErrInternalServer.WrapMsg(err.Error())
	}
	now := time.Now()
	seriesID := uuid.NewString()
	series := &chatdb.ScheduleMeetingSeries{
		ID: seriesID, MeetingID: meetingID, HostUserID: in.UserID,
		Title: title, CallType: in.CallType, Timezone: in.Timezone,
		DurationMinutes: dur, EnableWaitingRoom: in.EnableWaitingRoom,
		MuteOnJoin: muteOnJoin, AllowJoinBeforeMinutes: allowBefore,
		PasswordHash: hash, InviteeUserIDs: inviteeIDs,
		SourceGroupIDs: groupIDs, SourceConversationIDs: convIDs,
		RemindBeforeMinutes: reminds, Recurrence: in.Recurrence,
		Status: chatdb.ScheduleSeriesStatusActive, CreateTime: now, UpdateTime: now,
	}
	if series.Recurrence.Freq == "" {
		series.Recurrence.Freq = "none"
	}
	occs := make([]*chatdb.ScheduleMeetingOccurrence, 0, len(starts))
	for _, st := range starts {
		occs = append(occs, &chatdb.ScheduleMeetingOccurrence{
			ID: uuid.NewString(), SeriesID: seriesID, MeetingID: meetingID,
			StartTime: st, EndTime: st + int64(dur)*60*1000, OriginalStartTime: st,
			Status:     chatdb.ScheduleOccurrenceStatusScheduled,
			CreateTime: now, UpdateTime: now,
		})
	}
	if err := s.db.CreateSeries(ctx, series); err != nil {
		return nil, nil, err
	}
	if err := s.db.CreateOccurrences(ctx, occs); err != nil {
		_ = s.db.DeleteSeries(ctx, seriesID)
		return nil, nil, err
	}
	for _, occ := range occs {
		_ = s.reminders.ScheduleReminders(ctx, occ.ID, occ.StartTime, reminds)
	}
	if len(inviteeIDs) > 0 {
		s.notifier.NotifyInvitees(ctx, in.UserID, inviteeIDs, groupIDs, convIDs, ScheduleNotifyPayload{
			Action: "invited", MeetingID: meetingID, OccurrenceID: occs[0].ID, SeriesID: seriesID,
			Title: title, StartTime: occs[0].StartTime, EndTime: occs[0].EndTime,
			Timezone: in.Timezone, Scope: "this", OperatorUserID: in.UserID,
			HasPassword: hash != "",
		})
	}
	return series, occs[0], nil
}

func normalizeReminds(in []int) []int {
	if len(in) == 0 {
		return []int{15}
	}
	seen := map[int]struct{}{}
	var out []int
	for _, m := range in {
		if _, ok := allowedRemindMinutes[m]; !ok {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	if len(out) == 0 {
		return []int{15}
	}
	return out
}

func (s *ScheduleService) resolveInvitees(ctx context.Context, host string, inv ScheduleInvitees) (userIDs, groupIDs, convIDs []string, err error) {
	seen := map[string]struct{}{}
	add := func(uid string) {
		if uid == "" || uid == host {
			return
		}
		if _, ok := seen[uid]; ok {
			return
		}
		seen[uid] = struct{}{}
		userIDs = append(userIDs, uid)
	}
	for _, u := range inv.UserIDs {
		add(u)
	}
	groupSeen := map[string]struct{}{}
	for _, g := range inv.GroupIDs {
		if g == "" {
			continue
		}
		if _, ok := groupSeen[g]; ok {
			continue
		}
		groupSeen[g] = struct{}{}
		groupIDs = append(groupIDs, g)
		members, e := s.fetchGroupMembers(ctx, g)
		if e != nil {
			log.ZWarn(ctx, "fetch group members failed", e, "groupID", g)
			continue
		}
		for _, m := range members {
			add(m)
		}
	}
	convSeen := map[string]struct{}{}
	for _, cid := range inv.ConversationIDs {
		if cid == "" {
			continue
		}
		if _, ok := convSeen[cid]; ok {
			continue
		}
		convSeen[cid] = struct{}{}
		convIDs = append(convIDs, cid)
		if strings.HasPrefix(cid, "si_") {
			for _, p := range parseSiConversationUserIDs(cid, host) {
				add(p)
			}
		} else if strings.HasPrefix(cid, "sg_") {
			gid := strings.TrimPrefix(cid, "sg_")
			if _, ok := groupSeen[gid]; !ok {
				groupSeen[gid] = struct{}{}
				groupIDs = append(groupIDs, gid)
			}
			members, e := s.fetchGroupMembers(ctx, gid)
			if e != nil {
				continue
			}
			for _, m := range members {
				add(m)
			}
		}
	}
	if len(userIDs) > 500 {
		return nil, nil, nil, errs.ErrArgs.WrapMsg("too many invitees")
	}
	return userIDs, groupIDs, convIDs, nil
}

func (s *ScheduleService) fetchGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	if s.im == nil {
		return nil, fmt.Errorf("im unavailable")
	}
	tok, err := s.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		return nil, err
	}
	return s.im.GetGroupMemberUserIDs(mctx.WithApiToken(ctx, tok), groupID)
}

func effectiveTitle(series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence) string {
	if occ != nil && occ.Override != nil && occ.Override.Title != nil {
		return *occ.Override.Title
	}
	return series.Title
}

func effectiveInvitees(series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence) []string {
	if occ != nil && occ.Override != nil && len(occ.Override.InviteeUserIDs) > 0 {
		return occ.Override.InviteeUserIDs
	}
	return series.InviteeUserIDs
}

func effectiveDuration(series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence) int {
	if occ != nil && occ.Override != nil && occ.Override.DurationMinutes != nil {
		return *occ.Override.DurationMinutes
	}
	return series.DurationMinutes
}

func effectiveAllowBefore(series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence) int {
	if occ != nil && occ.Override != nil && occ.Override.AllowJoinBeforeMinutes != nil {
		return *occ.Override.AllowJoinBeforeMinutes
	}
	return series.AllowJoinBeforeMinutes
}

func effectiveWaiting(series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence) bool {
	if occ != nil && occ.Override != nil && occ.Override.EnableWaitingRoom != nil {
		return *occ.Override.EnableWaitingRoom
	}
	return series.EnableWaitingRoom
}

func effectiveMute(series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence) bool {
	if occ != nil && occ.Override != nil && occ.Override.MuteOnJoin != nil {
		return *occ.Override.MuteOnJoin
	}
	return series.MuteOnJoin
}

func effectiveReminds(series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence) []int {
	if occ != nil && occ.Override != nil && len(occ.Override.RemindBeforeMinutes) > 0 {
		return occ.Override.RemindBeforeMinutes
	}
	return series.RemindBeforeMinutes
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// parseSiConversationUserIDs 解析 si_{idA}_{idB}；userID 可含下划线时依赖 host 锚定两端。
func parseSiConversationUserIDs(conversationID, host string) []string {
	body := strings.TrimPrefix(conversationID, "si_")
	if body == "" {
		return nil
	}
	if host != "" {
		prefix := host + "_"
		if strings.HasPrefix(body, prefix) {
			peer := body[len(prefix):]
			if peer != "" {
				return []string{host, peer}
			}
		}
		suffix := "_" + host
		if strings.HasSuffix(body, suffix) {
			peer := body[:len(body)-len(suffix)]
			if peer != "" {
				return []string{peer, host}
			}
		}
	}
	// fallback：无 host 锚定时按首个 `_` 二分（兼容不含 `_` 的 userID）
	i := strings.Index(body, "_")
	if i <= 0 || i >= len(body)-1 {
		return nil
	}
	return []string{body[:i], body[i+1:]}
}

func (s *ScheduleService) requireHost(series *chatdb.ScheduleMeetingSeries, userID string) error {
	if series.HostUserID != userID {
		return errs.ErrNoPermission.WrapMsg("only host can operate")
	}
	return nil
}

// Get 详情（需调用方再做鉴权，或使用 GetAuthorized）
func (s *ScheduleService) Get(ctx context.Context, meetingID, occurrenceID string) (*chatdb.ScheduleMeetingSeries, *chatdb.ScheduleMeetingOccurrence, error) {
	var occ *chatdb.ScheduleMeetingOccurrence
	var err error
	if occurrenceID != "" {
		occ, err = s.db.GetOccurrence(ctx, occurrenceID)
		if err != nil {
			return nil, nil, err
		}
		meetingID = occ.MeetingID
	}
	series, err := s.db.GetSeriesByMeetingID(ctx, meetingID)
	if err != nil {
		return nil, nil, err
	}
	if occ == nil {
		now := time.Now().UnixMilli()
		occ, err = s.db.FindJoinableOccurrence(ctx, meetingID, now)
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, nil, err
		}
		if occ == nil {
			list, e := s.db.FindOccurrencesByMeetingID(ctx, meetingID)
			if e != nil {
				return nil, nil, e
			}
			if len(list) > 0 {
				occ = list[0]
			}
		}
	}
	return series, occ, nil
}

// GetAuthorized 详情并对非 host/invitee 拒绝。
func (s *ScheduleService) GetAuthorized(ctx context.Context, userID, meetingID, occurrenceID string) (*chatdb.ScheduleMeetingSeries, *chatdb.ScheduleMeetingOccurrence, error) {
	series, occ, err := s.Get(ctx, meetingID, occurrenceID)
	if err != nil {
		return nil, nil, err
	}
	if series == nil {
		return nil, nil, errs.ErrRecordNotFound.WrapMsg("meeting not found")
	}
	if series.HostUserID == userID {
		return series, occ, nil
	}
	for _, id := range series.InviteeUserIDs {
		if id == userID {
			return series, occ, nil
		}
	}
	if occ != nil {
		for _, id := range effectiveInvitees(series, occ) {
			if id == userID {
				return series, occ, nil
			}
		}
	}
	return nil, nil, errs.ErrNoPermission.WrapMsg("not a participant of this meeting")
}

// ListItem 列表项
type ListItem struct {
	Series     *chatdb.ScheduleMeetingSeries
	Occurrence *chatdb.ScheduleMeetingOccurrence
	IsHost     bool
}

// List 列表
func (s *ScheduleService) List(ctx context.Context, userID, role string, fromTime, toTime int64, pageNumber, showNumber int32) (int64, []ListItem, error) {
	hostIDs, _ := s.db.FindSeriesIDsByHost(ctx, userID)
	invIDs, _ := s.db.FindSeriesIDsByInvitee(ctx, userID)
	overrideIDs, _ := s.db.FindSeriesIDsByOverrideInvitee(ctx, userID)
	invIDs = uniqueStrings(append(invIDs, overrideIDs...))
	var seriesIDs []string
	switch role {
	case "host":
		seriesIDs = hostIDs
	case "invited":
		seriesIDs = invIDs
	default:
		seriesIDs = uniqueStrings(append(hostIDs, invIDs...))
		role = "all"
	}
	if pageNumber <= 0 {
		pageNumber = 1
	}
	if showNumber <= 0 {
		showNumber = 20
	}
	p := &simplePagination{pageNumber: pageNumber, showNumber: showNumber}
	total, occs, err := s.db.ListOccurrences(ctx, userID, seriesIDs, role, fromTime, toTime, p)
	if err != nil {
		return 0, nil, err
	}
	hostSet := map[string]struct{}{}
	for _, id := range hostIDs {
		hostSet[id] = struct{}{}
	}
	seriesCache := map[string]*chatdb.ScheduleMeetingSeries{}
	items := make([]ListItem, 0, len(occs))
	for _, occ := range occs {
		ser, ok := seriesCache[occ.SeriesID]
		if !ok {
			ser, err = s.db.GetSeries(ctx, occ.SeriesID)
			if err != nil {
				continue
			}
			seriesCache[occ.SeriesID] = ser
		}
		_, isHost := hostSet[occ.SeriesID]
		items = append(items, ListItem{Series: ser, Occurrence: occ, IsHost: isHost})
	}
	return total, items, nil
}

type simplePagination struct {
	pageNumber, showNumber int32
}

func (p *simplePagination) GetPageNumber() int32 { return p.pageNumber }
func (p *simplePagination) GetShowNumber() int32 { return p.showNumber }

// Cancel 取消预约
func (s *ScheduleService) Cancel(ctx context.Context, userID, occurrenceID, scope string) (int64, error) {
	occ, err := s.db.GetOccurrence(ctx, occurrenceID)
	if err != nil {
		return 0, err
	}
	series, err := s.db.GetSeries(ctx, occ.SeriesID)
	if err != nil {
		return 0, err
	}
	if err := s.requireHost(series, userID); err != nil {
		return 0, err
	}
	if occ.Status == chatdb.ScheduleOccurrenceStatusEnded {
		return 0, errs.ErrArgs.WrapMsg("ended occurrence cannot be cancelled")
	}
	if occ.Status == chatdb.ScheduleOccurrenceStatusOngoing {
		return 0, errs.ErrArgs.WrapMsg("ongoing occurrence should be ended, not cancelled")
	}
	if scope == "" {
		scope = "this"
	}
	var n int64
	switch scope {
	case "this":
		if occ.Status == chatdb.ScheduleOccurrenceStatusScheduled {
			occ.Status = chatdb.ScheduleOccurrenceStatusCancelled
			if err := s.db.UpdateOccurrence(ctx, occ); err != nil {
				return 0, err
			}
			s.reminders.RemoveOccurrenceReminders(ctx, occ.ID, effectiveReminds(series, occ))
			n = 1
		}
	case "this_and_following":
		n, err = s.db.CancelFrom(ctx, series.ID, occ.StartTime, true)
		if err != nil {
			return 0, err
		}
		s.clearRemindersFrom(ctx, series.ID, occ.StartTime, true)
	case "all":
		n, err = s.db.CancelFrom(ctx, series.ID, 0, true)
		if err != nil {
			return 0, err
		}
		series.Status = chatdb.ScheduleSeriesStatusCancelled
		_ = s.db.UpdateSeries(ctx, series)
		s.clearRemindersFrom(ctx, series.ID, 0, true)
	default:
		return 0, errs.ErrArgs.WrapMsg("invalid scope")
	}
	s.notifier.NotifyInvitees(ctx, userID, series.InviteeUserIDs, series.SourceGroupIDs, series.SourceConversationIDs, ScheduleNotifyPayload{
		Action: "cancelled", MeetingID: series.MeetingID, OccurrenceID: occ.ID, SeriesID: series.ID,
		Title: series.Title, StartTime: occ.StartTime, Timezone: series.Timezone, Scope: scope,
		OperatorUserID: userID, HasPassword: series.PasswordHash != "",
	})
	return n, nil
}

func (s *ScheduleService) clearRemindersFrom(ctx context.Context, seriesID string, fromStart int64, include bool) {
	list, err := s.db.FindOccurrencesBySeries(ctx, seriesID)
	if err != nil {
		return
	}
	for _, o := range list {
		if include {
			if o.StartTime < fromStart && fromStart > 0 {
				continue
			}
		} else if o.StartTime <= fromStart {
			continue
		}
		s.reminders.RemoveOccurrenceReminders(ctx, o.ID, nil)
	}
}

// Update 修改预约
func (s *ScheduleService) Update(ctx context.Context, in UpdateScheduleInput) (*chatdb.ScheduleMeetingSeries, *chatdb.ScheduleMeetingOccurrence, error) {
	occ, err := s.db.GetOccurrence(ctx, in.OccurrenceID)
	if err != nil {
		return nil, nil, err
	}
	series, err := s.db.GetSeries(ctx, occ.SeriesID)
	if err != nil {
		return nil, nil, err
	}
	if err := s.requireHost(series, in.UserID); err != nil {
		return nil, nil, err
	}
	if occ.Status == chatdb.ScheduleOccurrenceStatusEnded || occ.Status == chatdb.ScheduleOccurrenceStatusCancelled {
		return nil, nil, errs.ErrArgs.WrapMsg("cannot update ended/cancelled occurrence")
	}
	if scope := in.Scope; scope == "" {
		in.Scope = "this"
	}
	oldStart := occ.StartTime
	oldInvitees := append([]string(nil), series.InviteeUserIDs...)

	switch in.Scope {
	case "this":
		if err := s.updateThis(ctx, series, occ, in); err != nil {
			return nil, nil, err
		}
	case "this_and_following", "all":
		if err := s.updateSeriesScope(ctx, series, occ, in); err != nil {
			return nil, nil, err
		}
		// reload
		series, _ = s.db.GetSeries(ctx, series.ID)
		occ, _ = s.db.GetOccurrence(ctx, occ.ID)
	default:
		return nil, nil, errs.ErrArgs.WrapMsg("invalid scope")
	}

	newInvitees := series.InviteeUserIDs
	if occ.Override != nil && len(occ.Override.InviteeUserIDs) > 0 {
		newInvitees = occ.Override.InviteeUserIDs
	}
	added, removed := diffStrings(oldInvitees, newInvitees)
	s.notifier.NotifyInvitees(ctx, in.UserID, newInvitees, series.SourceGroupIDs, series.SourceConversationIDs, ScheduleNotifyPayload{
		Action: "updated", MeetingID: series.MeetingID, OccurrenceID: occ.ID, SeriesID: series.ID,
		Title: effectiveTitle(series, occ), StartTime: occ.StartTime, OldStartTime: oldStart,
		EndTime: occ.EndTime, Timezone: series.Timezone, Scope: in.Scope,
		OperatorUserID: in.UserID, HasPassword: series.PasswordHash != "",
	})
	if len(added) > 0 {
		s.notifier.NotifyInvitees(ctx, in.UserID, added, nil, nil, ScheduleNotifyPayload{
			Action: "invited", MeetingID: series.MeetingID, OccurrenceID: occ.ID, SeriesID: series.ID,
			Title: effectiveTitle(series, occ), StartTime: occ.StartTime, Timezone: series.Timezone,
			Scope: in.Scope, OperatorUserID: in.UserID, HasPassword: series.PasswordHash != "",
		})
	}
	if len(removed) > 0 {
		s.notifier.NotifyInvitees(ctx, in.UserID, removed, nil, nil, ScheduleNotifyPayload{
			Action: "removed", MeetingID: series.MeetingID, OccurrenceID: occ.ID, SeriesID: series.ID,
			Title: effectiveTitle(series, occ), StartTime: occ.StartTime, Timezone: series.Timezone,
			Scope: in.Scope, OperatorUserID: in.UserID,
		})
	}
	return series, occ, nil
}

func diffStrings(old, neu []string) (added, removed []string) {
	oldSet := map[string]struct{}{}
	newSet := map[string]struct{}{}
	for _, s := range old {
		oldSet[s] = struct{}{}
	}
	for _, s := range neu {
		newSet[s] = struct{}{}
		if _, ok := oldSet[s]; !ok {
			added = append(added, s)
		}
	}
	for _, s := range old {
		if _, ok := newSet[s]; !ok {
			removed = append(removed, s)
		}
	}
	return
}

func (s *ScheduleService) updateThis(ctx context.Context, series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence, in UpdateScheduleInput) error {
	if occ.Status == chatdb.ScheduleOccurrenceStatusOngoing && in.StartTime != nil {
		return errs.ErrArgs.WrapMsg("cannot change time of ongoing occurrence")
	}
	if occ.Override == nil {
		occ.Override = &chatdb.ScheduleOccurrenceOverride{}
	}
	ov := occ.Override
	if in.Title != nil {
		t, err := normalizeScheduleTitle(*in.Title)
		if err != nil {
			return err
		}
		ov.Title = &t
	}
	if in.DurationMinutes != nil {
		d := *in.DurationMinutes
		if d < 15 || d > 480 || d%15 != 0 {
			return errs.ErrArgs.WrapMsg("durationMinutes invalid")
		}
		ov.DurationMinutes = &d
		occ.EndTime = occ.StartTime + int64(d)*60*1000
	}
	if in.EnableWaitingRoom != nil {
		ov.EnableWaitingRoom = in.EnableWaitingRoom
	}
	if in.MuteOnJoin != nil {
		ov.MuteOnJoin = in.MuteOnJoin
	}
	if in.AllowJoinBeforeMinutes != nil {
		ov.AllowJoinBeforeMinutes = in.AllowJoinBeforeMinutes
	}
	remindChanged := false
	if in.RemindBeforeMinutes != nil {
		ov.RemindBeforeMinutes = normalizeReminds(in.RemindBeforeMinutes)
		remindChanged = true
	}
	if in.Invitees != nil {
		uids, _, _, err := s.resolveInvitees(ctx, series.HostUserID, *in.Invitees)
		if err != nil {
			return err
		}
		ov.InviteeUserIDs = uids
	}
	if in.StartTime != nil {
		occ.StartTime = *in.StartTime
		dur := effectiveDuration(series, occ)
		occ.EndTime = occ.StartTime + int64(dur)*60*1000
		remindChanged = true
	}
	if remindChanged {
		s.reminders.RemoveOccurrenceReminders(ctx, occ.ID, nil)
		_ = s.reminders.ScheduleReminders(ctx, occ.ID, occ.StartTime, effectiveReminds(series, occ))
	}
	if in.ClearPassword {
		series.PasswordHash = ""
		_ = s.db.UpdateSeries(ctx, series)
	} else if in.Password != nil {
		if err := ValidateMeetingPasswordFormat(*in.Password); err != nil {
			return errs.ErrArgs.WrapMsg(err.Error())
		}
		hash, err := HashMeetingPassword(*in.Password)
		if err != nil {
			return err
		}
		series.PasswordHash = hash
		_ = s.db.UpdateSeries(ctx, series)
	}
	return s.db.UpdateOccurrence(ctx, occ)
}

func (s *ScheduleService) updateSeriesScope(ctx context.Context, series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence, in UpdateScheduleInput) error {
	from := occ.StartTime
	if in.Scope == "all" {
		from = 0
	}
	// apply series field updates
	if in.Title != nil {
		t, err := normalizeScheduleTitle(*in.Title)
		if err != nil {
			return err
		}
		series.Title = t
	}
	if in.DurationMinutes != nil {
		d := *in.DurationMinutes
		if d < 15 || d > 480 || d%15 != 0 {
			return errs.ErrArgs.WrapMsg("durationMinutes invalid")
		}
		series.DurationMinutes = d
	}
	if in.Timezone != nil {
		if _, err := time.LoadLocation(*in.Timezone); err != nil {
			return errs.ErrArgs.WrapMsg("invalid timezone")
		}
		series.Timezone = *in.Timezone
	}
	if in.EnableWaitingRoom != nil {
		series.EnableWaitingRoom = *in.EnableWaitingRoom
	}
	if in.MuteOnJoin != nil {
		series.MuteOnJoin = *in.MuteOnJoin
	}
	if in.AllowJoinBeforeMinutes != nil {
		series.AllowJoinBeforeMinutes = *in.AllowJoinBeforeMinutes
	}
	remindOnly := false
	if in.RemindBeforeMinutes != nil {
		series.RemindBeforeMinutes = normalizeReminds(in.RemindBeforeMinutes)
		remindOnly = true
	}
	if in.Invitees != nil {
		uids, gids, cids, err := s.resolveInvitees(ctx, series.HostUserID, *in.Invitees)
		if err != nil {
			return err
		}
		series.InviteeUserIDs = uids
		series.SourceGroupIDs = gids
		series.SourceConversationIDs = cids
	}
	if in.ClearPassword {
		series.PasswordHash = ""
	} else if in.Password != nil {
		if err := ValidateMeetingPasswordFormat(*in.Password); err != nil {
			return errs.ErrArgs.WrapMsg(err.Error())
		}
		hash, err := HashMeetingPassword(*in.Password)
		if err != nil {
			return err
		}
		series.PasswordHash = hash
	}
	needRebuild := in.StartTime != nil || in.Recurrence != nil || in.Timezone != nil || in.DurationMinutes != nil
	if in.Recurrence != nil {
		series.Recurrence = *in.Recurrence
	}
	if err := s.db.UpdateSeries(ctx, series); err != nil {
		return err
	}
	if !needRebuild {
		if remindOnly {
			s.rescheduleRemindersFrom(ctx, series, from)
		}
		return nil
	}
	return s.rebuildOccurrencesInPlace(ctx, series, occ, from, in)
}

// rescheduleRemindersFrom 仅提醒变更时，按系列提醒配置重写受影响场次的 ZSET。
func (s *ScheduleService) rescheduleRemindersFrom(ctx context.Context, series *chatdb.ScheduleMeetingSeries, fromStart int64) {
	list, err := s.db.FindOccurrencesBySeries(ctx, series.ID)
	if err != nil {
		return
	}
	for _, o := range list {
		if o.Status != chatdb.ScheduleOccurrenceStatusScheduled {
			continue
		}
		if fromStart > 0 && o.StartTime < fromStart {
			continue
		}
		s.reminders.RemoveOccurrenceReminders(ctx, o.ID, nil)
		_ = s.reminders.ScheduleReminders(ctx, o.ID, o.StartTime, series.RemindBeforeMinutes)
	}
}

// rebuildOccurrencesInPlace 重建场次：尽量复用 scheduled 的 occurrenceID，避开与 ongoing/ended 的 start_time 冲突。
func (s *ScheduleService) rebuildOccurrencesInPlace(
	ctx context.Context, series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence,
	from int64, in UpdateScheduleInput,
) error {
	existing, err := s.db.FindOccurrencesBySeries(ctx, series.ID)
	if err != nil {
		return err
	}
	var reusable []*chatdb.ScheduleMeetingOccurrence
	blockedStarts := map[int64]struct{}{}
	for _, o := range existing {
		if o.Status == chatdb.ScheduleOccurrenceStatusScheduled {
			if from > 0 && o.StartTime < from {
				continue
			}
			reusable = append(reusable, o)
			continue
		}
		// ongoing/ended/cancelled occupy their start_time uniquely
		blockedStarts[o.StartTime] = struct{}{}
	}
	// Clear reminders before mutating/deleting.
	for _, o := range reusable {
		s.reminders.RemoveOccurrenceReminders(ctx, o.ID, nil)
	}

	firstStart := occ.StartTime
	if in.StartTime != nil {
		firstStart = *in.StartTime
	}
	if in.Scope == "all" {
		for _, o := range existing {
			if o.Status == chatdb.ScheduleOccurrenceStatusScheduled {
				firstStart = o.StartTime
				if in.StartTime != nil {
					firstStart = *in.StartTime
				}
				break
			}
		}
	}
	starts, err := ExpandOccurrenceStarts(firstStart, series.Timezone, series.Recurrence)
	if err != nil {
		return errs.ErrArgs.WrapMsg(err.Error())
	}
	filtered := make([]int64, 0, len(starts))
	for _, st := range starts {
		if in.Scope != "all" && from > 0 && st < from {
			continue
		}
		if _, blocked := blockedStarts[st]; blocked {
			continue
		}
		filtered = append(filtered, st)
	}
	if len(filtered) == 0 {
		return errs.ErrArgs.WrapMsg("recurrence produced zero occurrences")
	}

	now := time.Now()
	durMs := int64(series.DurationMinutes) * 60 * 1000
	nReuse := len(reusable)
	if nReuse > len(filtered) {
		nReuse = len(filtered)
	}
	var first *chatdb.ScheduleMeetingOccurrence
	for i := 0; i < nReuse; i++ {
		o := reusable[i]
		st := filtered[i]
		o.StartTime = st
		o.EndTime = st + durMs
		o.OriginalStartTime = st
		o.Override = nil
		o.UpdateTime = now
		if err := s.db.UpdateOccurrence(ctx, o); err != nil {
			return err
		}
		_ = s.reminders.ScheduleReminders(ctx, o.ID, o.StartTime, series.RemindBeforeMinutes)
		if first == nil {
			first = o
		}
	}
	var toCreate []*chatdb.ScheduleMeetingOccurrence
	for i := nReuse; i < len(filtered); i++ {
		st := filtered[i]
		toCreate = append(toCreate, &chatdb.ScheduleMeetingOccurrence{
			ID: uuid.NewString(), SeriesID: series.ID, MeetingID: series.MeetingID,
			StartTime: st, EndTime: st + durMs, OriginalStartTime: st,
			Status: chatdb.ScheduleOccurrenceStatusScheduled, CreateTime: now, UpdateTime: now,
		})
	}
	if len(toCreate) > 0 {
		if err := s.db.CreateOccurrences(ctx, toCreate); err != nil {
			return err
		}
		for _, o := range toCreate {
			_ = s.reminders.ScheduleReminders(ctx, o.ID, o.StartTime, series.RemindBeforeMinutes)
			if first == nil {
				first = o
			}
		}
	}
	// Cancel leftover reusable scheduled rows (no longer needed).
	for i := nReuse; i < len(reusable); i++ {
		o := reusable[i]
		o.Status = chatdb.ScheduleOccurrenceStatusCancelled
		o.UpdateTime = now
		_ = s.db.UpdateOccurrence(ctx, o)
	}
	if first != nil {
		*occ = *first
	}
	return nil
}

// End 组织者标记本场结束
func (s *ScheduleService) End(ctx context.Context, userID, meetingID, occurrenceID string) (*chatdb.ScheduleMeetingOccurrence, error) {
	var occ *chatdb.ScheduleMeetingOccurrence
	var err error
	if occurrenceID != "" {
		occ, err = s.db.GetOccurrence(ctx, occurrenceID)
	} else {
		occ, err = s.db.FindJoinableOccurrence(ctx, meetingID, time.Now().UnixMilli())
		if err == mongo.ErrNoDocuments {
			list, e := s.db.FindOccurrencesByMeetingID(ctx, meetingID)
			if e != nil {
				return nil, e
			}
			for _, o := range list {
				if o.Status == chatdb.ScheduleOccurrenceStatusOngoing || o.Status == chatdb.ScheduleOccurrenceStatusScheduled {
					occ = o
					break
				}
			}
			if occ == nil {
				return nil, errs.ErrRecordNotFound.WrapMsg("occurrence not found")
			}
			err = nil
		}
	}
	if err != nil {
		return nil, err
	}
	series, err := s.db.GetSeries(ctx, occ.SeriesID)
	if err != nil {
		return nil, err
	}
	if err := s.requireHost(series, userID); err != nil {
		return nil, err
	}
	if occ.Status == chatdb.ScheduleOccurrenceStatusEnded {
		return occ, nil
	}
	if occ.Status == chatdb.ScheduleOccurrenceStatusCancelled {
		return nil, errs.ErrArgs.WrapMsg("cancelled occurrence")
	}
	occ.Status = chatdb.ScheduleOccurrenceStatusEnded
	occ.EndedAt = time.Now().UnixMilli()
	occ.EndedByUserID = userID
	occ.EndReason = chatdb.ScheduleEndReasonHostComplete
	if err := s.db.UpdateOccurrence(ctx, occ); err != nil {
		return nil, err
	}
	s.reminders.RemoveOccurrenceReminders(ctx, occ.ID, effectiveReminds(series, occ))
	if s.conf != nil && s.conf.RoomExists(series.MeetingID) {
		_ = s.conf.EndConference(series.MeetingID, userID)
	}
	if s.waiting != nil {
		s.waiting.Clear(ctx, series.MeetingID)
	}
	return occ, nil
}

// Start 主持人强制激活
func (s *ScheduleService) Start(ctx context.Context, userID, occurrenceID string) (*JoinConferenceResult, error) {
	occ, err := s.db.GetOccurrence(ctx, occurrenceID)
	if err != nil {
		return nil, err
	}
	series, err := s.db.GetSeries(ctx, occ.SeriesID)
	if err != nil {
		return nil, err
	}
	if err := s.requireHost(series, userID); err != nil {
		return nil, err
	}
	if occ.Status == chatdb.ScheduleOccurrenceStatusEnded {
		return nil, errs.ErrArgs.WrapMsg("本次会议已经结束")
	}
	if occ.Status == chatdb.ScheduleOccurrenceStatusCancelled {
		return nil, errs.ErrArgs.WrapMsg("meeting cancelled")
	}
	if err := s.ensureActivated(ctx, series, occ, true); err != nil {
		return nil, err
	}
	res, err := s.conf.JoinConferenceWithOpts(series.MeetingID, userID, "", true)
	if err != nil {
		return nil, err
	}
	s.admitAllWaitingAndNotify(ctx, series.HostUserID, series.MeetingID)
	return res, nil
}

// Join 入会（含密码/等候室/结束校验）
func (s *ScheduleService) Join(ctx context.Context, userID, meetingID, occurrenceID, password string) (*JoinConferenceResult, *chatdb.ScheduleMeetingOccurrence, error) {
	var occ *chatdb.ScheduleMeetingOccurrence
	var err error
	if occurrenceID != "" {
		occ, err = s.db.GetOccurrence(ctx, occurrenceID)
	} else {
		occ, err = s.db.FindJoinableOccurrence(ctx, meetingID, time.Now().UnixMilli())
		if err == mongo.ErrNoDocuments {
			occ, err = s.pickOccurrenceForJoin(ctx, meetingID)
		}
	}
	if err != nil {
		return nil, occ, err
	}
	if occ == nil {
		return nil, nil, errs.ErrRecordNotFound.WrapMsg("occurrence not found")
	}
	series, err := s.db.GetSeries(ctx, occ.SeriesID)
	if err != nil {
		return nil, nil, err
	}
	if occ.Status == chatdb.ScheduleOccurrenceStatusEnded {
		return nil, occ, errs.ErrArgs.WrapMsg("本次会议已经结束")
	}
	if occ.Status == chatdb.ScheduleOccurrenceStatusCancelled {
		return nil, occ, errs.ErrArgs.WrapMsg("meeting cancelled")
	}
	now := time.Now().UnixMilli()
	isHost := userID == series.HostUserID
	allowBefore := effectiveAllowBefore(series, occ)
	earliest := occ.StartTime - int64(allowBefore)*60*1000
	if isHost {
		earliest = occ.StartTime - int64(max(allowBefore, 60))*60*1000
	}
	graceEnd := occ.EndTime + 30*60*1000
	if now < earliest {
		return nil, occ, errs.ErrArgs.WrapMsg("too_early")
	}
	if now > graceEnd && occ.Status != chatdb.ScheduleOccurrenceStatusOngoing {
		return nil, occ, errs.ErrArgs.WrapMsg("本次会议已经结束")
	}
	passwordVerified := isHost
	// Host is exempt from meeting password.
	if series.PasswordHash != "" && !isHost {
		if password == "" {
			return nil, occ, errs.ErrArgs.WrapMsg("password_required")
		}
		if s.pwdLimiter != nil {
			ok, _ := s.pwdLimiter.CheckAllowed(ctx, series.MeetingID, userID)
			if !ok {
				return nil, occ, errs.ErrArgs.WrapMsg("password_rate_limited")
			}
		}
		if !VerifyMeetingPassword(series.PasswordHash, password) {
			if s.pwdLimiter != nil {
				s.pwdLimiter.RecordFailure(ctx, series.MeetingID, userID)
			}
			return nil, occ, errs.ErrArgs.WrapMsg("wrong_password")
		}
		if s.pwdLimiter != nil {
			s.pwdLimiter.ClearFailures(ctx, series.MeetingID, userID)
		}
		passwordVerified = true
	} else if series.PasswordHash == "" {
		passwordVerified = true
	}

	// 非主持人：房间尚未激活时一律进入等候，禁止 ensureActivated 建房。
	if !isHost && (s.conf == nil || !s.conf.RoomExists(series.MeetingID)) {
		if s.waiting == nil {
			return nil, occ, errs.ErrArgs.WrapMsg("waiting room unavailable")
		}
		admitted, _ := s.waiting.IsAdmitted(ctx, series.MeetingID, userID)
		if admitted {
			return &JoinConferenceResult{Status: "waiting"}, occ, nil
		}
		nickname, face := "", ""
		if s.im != nil {
			if tok, e := s.im.ImAdminTokenWithDefaultAdmin(ctx); e == nil {
				if u, e2 := s.im.GetUserInfo(mctx.WithApiToken(ctx, tok), userID); e2 == nil && u != nil {
					nickname, face = u.Nickname, u.FaceURL
				}
			}
		}
		count, already, e := s.waiting.Enqueue(ctx, series.MeetingID, userID, nickname, face)
		if e != nil {
			return nil, occ, errs.ErrArgs.WrapMsg(e.Error())
		}
		if already {
			return &JoinConferenceResult{Status: "waiting"}, occ, nil
		}
		s.waiting.NotifyHostWaiting(ctx, series.HostUserID, series.MeetingID, "waiting_updated", []string{userID})
		return &JoinConferenceResult{Status: "waiting", WaitingCount: count}, occ, nil
	}

	if err := s.ensureActivated(ctx, series, occ, isHost); err != nil {
		return nil, occ, err
	}
	res, err := s.conf.JoinConferenceWithOpts(series.MeetingID, userID, password, passwordVerified)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "password") || strings.Contains(msg, "waiting") || strings.Contains(msg, "locked") {
			return nil, occ, errs.ErrArgs.WrapMsg(msg)
		}
		return nil, occ, errs.ErrInternalServer.WrapMsg(msg)
	}
	if isHost {
		s.admitAllWaitingAndNotify(ctx, series.HostUserID, series.MeetingID)
	}
	return res, occ, nil
}

// pickOccurrenceForJoin 在 FindJoinable 未命中时选择场次：优先 ongoing/未来 scheduled，避免误报已结束。
func (s *ScheduleService) pickOccurrenceForJoin(ctx context.Context, meetingID string) (*chatdb.ScheduleMeetingOccurrence, error) {
	list, err := s.db.FindOccurrencesByMeetingID(ctx, meetingID)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, errs.ErrRecordNotFound.WrapMsg("occurrence not found")
	}
	var ongoing, nextScheduled, recentEnded *chatdb.ScheduleMeetingOccurrence
	for _, o := range list {
		switch o.Status {
		case chatdb.ScheduleOccurrenceStatusOngoing:
			if ongoing == nil || o.StartTime > ongoing.StartTime {
				ongoing = o
			}
		case chatdb.ScheduleOccurrenceStatusScheduled:
			if nextScheduled == nil || o.StartTime < nextScheduled.StartTime {
				nextScheduled = o
			}
		case chatdb.ScheduleOccurrenceStatusEnded:
			if recentEnded == nil || o.EndTime > recentEnded.EndTime {
				recentEnded = o
			}
		}
	}
	if ongoing != nil {
		return ongoing, nil
	}
	if nextScheduled != nil {
		// Fall through to Join time-window checks (may return too_early).
		return nextScheduled, nil
	}
	if recentEnded != nil {
		return recentEnded, errs.ErrArgs.WrapMsg("本次会议已经结束")
	}
	return nil, errs.ErrRecordNotFound.WrapMsg("occurrence not found")
}

func (s *ScheduleService) ensureActivated(ctx context.Context, series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence, _ bool) error {
	if s.conf == nil {
		return errs.ErrInternalServer.WrapMsg("ConferenceManager is not configured")
	}
	if s.conf.RoomExists(series.MeetingID) {
		if occ.Status == chatdb.ScheduleOccurrenceStatusScheduled {
			occ.Status = chatdb.ScheduleOccurrenceStatusOngoing
			occ.ActivatedRoom = true
			return s.db.UpdateOccurrence(ctx, occ)
		}
		return nil
	}
	groupID := ""
	if len(series.SourceGroupIDs) > 0 {
		groupID = series.SourceGroupIDs[0]
	}
	convID := ""
	if len(series.SourceConversationIDs) > 0 {
		convID = series.SourceConversationIDs[0]
	}
	_, _, err := s.conf.CreateConferenceOpts(CreateConferenceOpts{
		RoomID:               series.MeetingID,
		CreatorUserID:        series.HostUserID,
		CallType:             series.CallType,
		Title:                effectiveTitle(series, occ),
		InviteeUserIDs:       effectiveInvitees(series, occ),
		GroupID:              groupID,
		ConversationID:       convID,
		PasswordHash:         series.PasswordHash,
		WaitingRoomEnabled:   effectiveWaiting(series, occ),
		MuteOnJoin:           effectiveMute(series, occ),
		ScheduleOccurrenceID: occ.ID,
		SkipInvites:          true,
		SkipAnnounce:         true,
		SkipAddCreator:       true,
	})
	if err != nil {
		return err
	}
	if occ.Status == chatdb.ScheduleOccurrenceStatusScheduled {
		occ.Status = chatdb.ScheduleOccurrenceStatusOngoing
		occ.ActivatedRoom = true
		if err := s.db.UpdateOccurrence(ctx, occ); err != nil {
			return err
		}
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// admitAllWaitingAndNotify 主持人开始/激活会议后，准入等候成员并推送 waiting_admitted。
// 同时通知已手动准入但尚未入会的成员（房间创建前 admitted 的场景）。
func (s *ScheduleService) admitAllWaitingAndNotify(ctx context.Context, hostUserID, meetingID string) {
	if s.waiting == nil || meetingID == "" {
		return
	}
	list, err := s.waiting.List(ctx, meetingID)
	if err == nil && len(list) > 0 {
		ids := make([]string, 0, len(list))
		for _, u := range list {
			ids = append(ids, u.UserID)
		}
		_ = s.waiting.Admit(ctx, meetingID, ids)
	}
	admitted, err := s.waiting.ListAdmitted(ctx, meetingID)
	if err != nil || len(admitted) == 0 {
		return
	}
	s.waiting.NotifyHostWaiting(ctx, hostUserID, meetingID, "waiting_admitted", admitted)
}
