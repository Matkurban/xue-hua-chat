package chat

import (
	"context"
	"time"

	"chat/pkg/common/db/pagination"
)

// 预约系列状态
const (
	ScheduleSeriesStatusActive    = "active"
	ScheduleSeriesStatusCancelled = "cancelled"
)

// 预约场次状态
const (
	ScheduleOccurrenceStatusScheduled = "scheduled"
	ScheduleOccurrenceStatusOngoing   = "ongoing"
	ScheduleOccurrenceStatusEnded     = "ended"
	ScheduleOccurrenceStatusCancelled = "cancelled"
)

// 结束原因
const (
	ScheduleEndReasonHostComplete = "host_complete"
	ScheduleEndReasonRoomEnded    = "room_ended"
	ScheduleEndReasonExpired      = "expired"
)

// ScheduleRecurrence 重复规则（RRULE 子集）
type ScheduleRecurrence struct {
	Freq       string `bson:"freq" json:"freq"`                        // none|daily|weekly|monthly
	Interval   int    `bson:"interval" json:"interval"`                // 默认 1
	ByWeekdays []int  `bson:"by_weekdays,omitempty" json:"byWeekdays"` // 1=周一 … 7=周日
	ByMonthDay int    `bson:"by_month_day,omitempty" json:"byMonthDay"`
	Until      int64  `bson:"until,omitempty" json:"until"` // UTC ms
	Count      int    `bson:"count,omitempty" json:"count"`
}

// ScheduleOccurrenceOverride 单场覆盖字段
type ScheduleOccurrenceOverride struct {
	Title                  *string  `bson:"title,omitempty" json:"title,omitempty"`
	DurationMinutes        *int     `bson:"duration_minutes,omitempty" json:"durationMinutes,omitempty"`
	EnableWaitingRoom      *bool    `bson:"enable_waiting_room,omitempty" json:"enableWaitingRoom,omitempty"`
	MuteOnJoin             *bool    `bson:"mute_on_join,omitempty" json:"muteOnJoin,omitempty"`
	AllowJoinBeforeMinutes *int     `bson:"allow_join_before_minutes,omitempty" json:"allowJoinBeforeMinutes,omitempty"`
	InviteeUserIDs         []string `bson:"invitee_user_ids,omitempty" json:"inviteeUserIDs,omitempty"`
	RemindBeforeMinutes    []int    `bson:"remind_before_minutes,omitempty" json:"remindBeforeMinutes,omitempty"`
}

// ScheduleMeetingSeries 预约会议系列
type ScheduleMeetingSeries struct {
	ID                     string             `bson:"_id"`
	MeetingID              string             `bson:"meeting_id"`
	HostUserID             string             `bson:"host_user_id"`
	Title                  string             `bson:"title"`
	CallType               string             `bson:"call_type"`
	Timezone               string             `bson:"timezone"`
	DurationMinutes        int                `bson:"duration_minutes"`
	EnableWaitingRoom      bool               `bson:"enable_waiting_room"`
	MuteOnJoin             bool               `bson:"mute_on_join"`
	AllowJoinBeforeMinutes int                `bson:"allow_join_before_minutes"`
	PasswordHash           string             `bson:"password_hash,omitempty"`
	InviteeUserIDs         []string           `bson:"invitee_user_ids"`
	SourceGroupIDs         []string           `bson:"source_group_ids,omitempty"`
	SourceConversationIDs  []string           `bson:"source_conversation_ids,omitempty"`
	RemindBeforeMinutes    []int              `bson:"remind_before_minutes"`
	Recurrence             ScheduleRecurrence `bson:"recurrence"`
	Status                 string             `bson:"status"`
	CreateTime             time.Time          `bson:"create_time"`
	UpdateTime             time.Time          `bson:"update_time"`
}

func (ScheduleMeetingSeries) TableName() string { return "schedule_meeting_series" }

// ScheduleMeetingOccurrence 预约场次
type ScheduleMeetingOccurrence struct {
	ID                string                      `bson:"_id"`
	SeriesID          string                      `bson:"series_id"`
	MeetingID         string                      `bson:"meeting_id"`
	StartTime         int64                       `bson:"start_time"` // UTC ms
	EndTime           int64                       `bson:"end_time"`
	OriginalStartTime int64                       `bson:"original_start_time"`
	Status            string                      `bson:"status"`
	EndedAt           int64                       `bson:"ended_at,omitempty"`
	EndedByUserID     string                      `bson:"ended_by_user_id,omitempty"`
	EndReason         string                      `bson:"end_reason,omitempty"`
	Override          *ScheduleOccurrenceOverride `bson:"override,omitempty"`
	ActivatedRoom     bool                        `bson:"activated_room"`
	CreateTime        time.Time                   `bson:"create_time"`
	UpdateTime        time.Time                   `bson:"update_time"`
}

func (ScheduleMeetingOccurrence) TableName() string { return "schedule_meeting_occurrence" }

// ScheduleMeetingSeriesInterface 系列 CRUD
type ScheduleMeetingSeriesInterface interface {
	Create(ctx context.Context, series *ScheduleMeetingSeries) error
	Take(ctx context.Context, id string) (*ScheduleMeetingSeries, error)
	TakeByMeetingID(ctx context.Context, meetingID string) (*ScheduleMeetingSeries, error)
	Update(ctx context.Context, series *ScheduleMeetingSeries) error
	ExistsMeetingID(ctx context.Context, meetingID string) (bool, error)
	FindIDsByHost(ctx context.Context, hostUserID string) ([]string, error)
	FindIDsByInvitee(ctx context.Context, userID string) ([]string, error)
	Delete(ctx context.Context, id string) error
}

// ScheduleMeetingOccurrenceInterface 场次 CRUD
type ScheduleMeetingOccurrenceInterface interface {
	CreateMany(ctx context.Context, list []*ScheduleMeetingOccurrence) error
	Take(ctx context.Context, id string) (*ScheduleMeetingOccurrence, error)
	Update(ctx context.Context, occ *ScheduleMeetingOccurrence) error
	FindBySeries(ctx context.Context, seriesID string) ([]*ScheduleMeetingOccurrence, error)
	FindByMeetingID(ctx context.Context, meetingID string) ([]*ScheduleMeetingOccurrence, error)
	DeleteFutureScheduled(ctx context.Context, seriesID string, fromStartTime int64) (int64, error)
	CancelFrom(ctx context.Context, seriesID string, fromStartTime int64, includeThis bool) (int64, error)
	ListForUser(ctx context.Context, userID string, hostSeriesIDs []string, role string, fromTime, toTime int64, p pagination.Pagination) (int64, []*ScheduleMeetingOccurrence, error)
	FindJoinable(ctx context.Context, meetingID string, nowMs int64) (*ScheduleMeetingOccurrence, error)
	FindSeriesIDsByOverrideInvitee(ctx context.Context, userID string) ([]string, error)
	DeleteBySeries(ctx context.Context, seriesID string) error
}
