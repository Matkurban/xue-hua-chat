package chat

import (
	"gitee.com/matkurban/openimsdk-tools/errs"
)

type ScheduleInviteesReq struct {
	UserIDs         []string `json:"userIDs"`
	GroupIDs        []string `json:"groupIDs"`
	ConversationIDs []string `json:"conversationIDs"`
}

type ScheduleRecurrenceReq struct {
	Freq       string `json:"freq"`
	Interval   int    `json:"interval"`
	ByWeekdays []int  `json:"byWeekdays"`
	ByMonthDay int    `json:"byMonthDay"`
	Until      int64  `json:"until"`
	Count      int    `json:"count"`
}

type CreateScheduleMeetingReq struct {
	UserID                 string                `json:"userID"`
	Title                  string                `json:"title"`
	CallType               string                `json:"callType"`
	StartTime              int64                 `json:"startTime"`
	DurationMinutes        int                   `json:"durationMinutes"`
	Timezone               string                `json:"timezone"`
	EnableWaitingRoom      bool                  `json:"enableWaitingRoom"`
	MuteOnJoin             *bool                 `json:"muteOnJoin"`
	AllowJoinBeforeMinutes *int                  `json:"allowJoinBeforeMinutes"`
	RemindBeforeMinutes    []int                 `json:"remindBeforeMinutes"`
	Invitees               ScheduleInviteesReq   `json:"invitees"`
	Recurrence             ScheduleRecurrenceReq `json:"recurrence"`
	Password               string                `json:"password"`
}

func (x *CreateScheduleMeetingReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID is empty")
	}
	return nil
}

type CreateScheduleMeetingResp struct {
	MeetingID         string `json:"meetingID"`
	SeriesID          string `json:"seriesID"`
	OccurrenceID      string `json:"occurrenceID"`
	StartTime         int64  `json:"startTime"`
	EndTime           int64  `json:"endTime"`
	Timezone          string `json:"timezone"`
	Title             string `json:"title"`
	Status            string `json:"status"`
	HasPassword       bool   `json:"hasPassword"`
	EnableWaitingRoom bool   `json:"enableWaitingRoom"`
}

type UpdateScheduleMeetingReq struct {
	UserID                 string                 `json:"userID"`
	OccurrenceID           string                 `json:"occurrenceID"`
	Scope                  string                 `json:"scope"`
	Title                  *string                `json:"title"`
	StartTime              *int64                 `json:"startTime"`
	DurationMinutes        *int                   `json:"durationMinutes"`
	Timezone               *string                `json:"timezone"`
	EnableWaitingRoom      *bool                  `json:"enableWaitingRoom"`
	MuteOnJoin             *bool                  `json:"muteOnJoin"`
	AllowJoinBeforeMinutes *int                   `json:"allowJoinBeforeMinutes"`
	RemindBeforeMinutes    []int                  `json:"remindBeforeMinutes"`
	Invitees               *ScheduleInviteesReq   `json:"invitees"`
	Password               *string                `json:"password"`
	ClearPassword          bool                   `json:"clearPassword"`
	Recurrence             *ScheduleRecurrenceReq `json:"recurrence"`
}

func (x *UpdateScheduleMeetingReq) Check() error {
	if x.UserID == "" || x.OccurrenceID == "" {
		return errs.ErrArgs.WrapMsg("userID and occurrenceID required")
	}
	return nil
}

type CancelScheduleMeetingReq struct {
	UserID       string `json:"userID"`
	OccurrenceID string `json:"occurrenceID"`
	Scope        string `json:"scope"`
}

func (x *CancelScheduleMeetingReq) Check() error {
	if x.UserID == "" || x.OccurrenceID == "" {
		return errs.ErrArgs.WrapMsg("userID and occurrenceID required")
	}
	return nil
}

type CancelScheduleMeetingResp struct {
	Ok             bool  `json:"ok"`
	CancelledCount int64 `json:"cancelledCount"`
}

type ListScheduleMeetingReq struct {
	UserID     string `json:"userID"`
	Role       string `json:"role"`
	FromTime   int64  `json:"fromTime"`
	ToTime     int64  `json:"toTime"`
	PageNumber int32  `json:"pageNumber"`
	ShowNumber int32  `json:"showNumber"`
}

func (x *ListScheduleMeetingReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID is empty")
	}
	return nil
}

type ScheduleMeetingListItem struct {
	MeetingID      string `json:"meetingID"`
	OccurrenceID   string `json:"occurrenceID"`
	SeriesID       string `json:"seriesID"`
	Title          string `json:"title"`
	Timezone       string `json:"timezone"`
	Status         string `json:"status"`
	RecurrenceFreq string `json:"recurrenceFreq"`
	CallType       string `json:"callType"`
	StartTime      int64  `json:"startTime"`
	EndTime        int64  `json:"endTime"`
	IsHost         bool   `json:"isHost"`
	HasPassword    bool   `json:"hasPassword"`
}

type ListScheduleMeetingResp struct {
	Total int64                     `json:"total"`
	List  []ScheduleMeetingListItem `json:"list"`
}

type GetScheduleMeetingReq struct {
	UserID       string `json:"userID"`
	MeetingID    string `json:"meetingID"`
	OccurrenceID string `json:"occurrenceID"`
}

func (x *GetScheduleMeetingReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID is empty")
	}
	if x.MeetingID == "" && x.OccurrenceID == "" {
		return errs.ErrArgs.WrapMsg("meetingID or occurrenceID required")
	}
	return nil
}

type GetScheduleMeetingResp struct {
	MeetingID              string   `json:"meetingID"`
	SeriesID               string   `json:"seriesID"`
	OccurrenceID           string   `json:"occurrenceID"`
	Title                  string   `json:"title"`
	CallType               string   `json:"callType"`
	Timezone               string   `json:"timezone"`
	Status                 string   `json:"status"`
	HostUserID             string   `json:"hostUserID"`
	RecurrenceFreq         string   `json:"recurrenceFreq"`
	RemindBeforeMinutes    []int    `json:"remindBeforeMinutes"`
	InviteeUserIDs         []string `json:"inviteeUserIDs"`
	StartTime              int64    `json:"startTime"`
	EndTime                int64    `json:"endTime"`
	DurationMinutes        int      `json:"durationMinutes"`
	AllowJoinBeforeMinutes int      `json:"allowJoinBeforeMinutes"`
	EnableWaitingRoom      bool     `json:"enableWaitingRoom"`
	MuteOnJoin             bool     `json:"muteOnJoin"`
	HasPassword            bool     `json:"hasPassword"`
}

type JoinScheduleMeetingReq struct {
	UserID       string `json:"userID"`
	MeetingID    string `json:"meetingID"`
	OccurrenceID string `json:"occurrenceID"`
	Password     string `json:"password"`
}

func (x *JoinScheduleMeetingReq) Check() error {
	if x.UserID == "" || (x.MeetingID == "" && x.OccurrenceID == "") {
		return errs.ErrArgs.WrapMsg("userID and meetingID or occurrenceID required")
	}
	return nil
}

type JoinScheduleMeetingResp struct {
	Status       string `json:"status"`
	RoomID       string `json:"roomID,omitempty"`
	Token        string `json:"token,omitempty"`
	LiveURL      string `json:"liveURL,omitempty"`
	Role         string `json:"role,omitempty"`
	Title        string `json:"title,omitempty"`
	CallType     string `json:"callType,omitempty"`
	OccurrenceID string `json:"occurrenceID"`
	MeetingID    string `json:"meetingID"`
	WaitingCount int64  `json:"waitingCount,omitempty"`
}

type StartScheduleMeetingReq struct {
	UserID       string `json:"userID"`
	OccurrenceID string `json:"occurrenceID"`
}

func (x *StartScheduleMeetingReq) Check() error {
	if x.UserID == "" || x.OccurrenceID == "" {
		return errs.ErrArgs.WrapMsg("userID and occurrenceID required")
	}
	return nil
}

type EndScheduleMeetingReq struct {
	UserID       string `json:"userID"`
	MeetingID    string `json:"meetingID"`
	OccurrenceID string `json:"occurrenceID"`
}

func (x *EndScheduleMeetingReq) Check() error {
	if x.UserID == "" || (x.MeetingID == "" && x.OccurrenceID == "") {
		return errs.ErrArgs.WrapMsg("userID and meetingID or occurrenceID required")
	}
	return nil
}

type EndScheduleMeetingResp struct {
	Ok           bool   `json:"ok"`
	MeetingID    string `json:"meetingID"`
	OccurrenceID string `json:"occurrenceID"`
	Status       string `json:"status"`
	EndedAt      int64  `json:"endedAt"`
}

type WaitingListReq struct {
	UserID    string `json:"userID"`
	MeetingID string `json:"meetingID"`
}

func (x *WaitingListReq) Check() error {
	if x.UserID == "" || x.MeetingID == "" {
		return errs.ErrArgs.WrapMsg("userID and meetingID required")
	}
	return nil
}

type WaitingUserItem struct {
	UserID    string `json:"userID"`
	Nickname  string `json:"nickname,omitempty"`
	FaceURL   string `json:"faceURL,omitempty"`
	EnqueueAt int64  `json:"enqueueAt"`
}

type WaitingListResp struct {
	List []WaitingUserItem `json:"list"`
}

type WaitingAdmitReq struct {
	UserID        string   `json:"userID"`
	MeetingID     string   `json:"meetingID"`
	TargetUserIDs []string `json:"targetUserIDs"`
}

func (x *WaitingAdmitReq) Check() error {
	if x.UserID == "" || x.MeetingID == "" {
		return errs.ErrArgs.WrapMsg("userID and meetingID required")
	}
	return nil
}

type WaitingRejectReq struct {
	UserID        string   `json:"userID"`
	MeetingID     string   `json:"meetingID"`
	TargetUserIDs []string `json:"targetUserIDs"`
}

func (x *WaitingRejectReq) Check() error {
	if x.UserID == "" || x.MeetingID == "" {
		return errs.ErrArgs.WrapMsg("userID and meetingID required")
	}
	return nil
}

type WaitingAdmitAllReq struct {
	UserID    string `json:"userID"`
	MeetingID string `json:"meetingID"`
}

func (x *WaitingAdmitAllReq) Check() error {
	if x.UserID == "" || x.MeetingID == "" {
		return errs.ErrArgs.WrapMsg("userID and meetingID required")
	}
	return nil
}

type LeaveWaitingReq struct {
	UserID    string `json:"userID"`
	MeetingID string `json:"meetingID"`
}

func (x *LeaveWaitingReq) Check() error {
	if x.UserID == "" || x.MeetingID == "" {
		return errs.ErrArgs.WrapMsg("userID and meetingID required")
	}
	return nil
}
