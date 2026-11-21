package chat

import (
	chatdb "chat/pkg/common/db/table/chat"
	"chat/pkg/common/rtc"
	"chat/pkg/protocol/chat"
	"chat/pkg/common/apiresp"
	"chat/pkg/common/errs"
	"github.com/gin-gonic/gin"
)

func (o *Api) scheduleOrErr(c *gin.Context) bool {
	if o.ScheduleService == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("ScheduleService is not configured"))
		return false
	}
	return true
}

func toRecurrence(r chat.ScheduleRecurrenceReq) chatdb.ScheduleRecurrence {
	return chatdb.ScheduleRecurrence{
		Freq: r.Freq, Interval: r.Interval, ByWeekdays: r.ByWeekdays,
		ByMonthDay: r.ByMonthDay, Until: r.Until, Count: r.Count,
	}
}

func toInvitees(r chat.ScheduleInviteesReq) rtc.ScheduleInvitees {
	return rtc.ScheduleInvitees{
		UserIDs: r.UserIDs, GroupIDs: r.GroupIDs, ConversationIDs: r.ConversationIDs,
	}
}

func (o *Api) CreateScheduleMeeting(c *gin.Context) {
	var req chat.CreateScheduleMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.scheduleOrErr(c) {
		return
	}
	series, occ, err := o.ScheduleService.Create(c, rtc.CreateScheduleInput{
		UserID: req.UserID, Title: req.Title, CallType: req.CallType,
		StartTime: req.StartTime, DurationMinutes: req.DurationMinutes, Timezone: req.Timezone,
		EnableWaitingRoom: req.EnableWaitingRoom, MuteOnJoin: req.MuteOnJoin,
		AllowJoinBeforeMinutes: req.AllowJoinBeforeMinutes, RemindBeforeMinutes: req.RemindBeforeMinutes,
		Invitees: toInvitees(req.Invitees), Recurrence: toRecurrence(req.Recurrence), Password: req.Password,
	})
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &chat.CreateScheduleMeetingResp{
		MeetingID: series.MeetingID, SeriesID: series.ID, OccurrenceID: occ.ID,
		StartTime: occ.StartTime, EndTime: occ.EndTime, Timezone: series.Timezone,
		Title: series.Title, Status: occ.Status, HasPassword: series.PasswordHash != "",
		EnableWaitingRoom: series.EnableWaitingRoom,
	})
}

func (o *Api) UpdateScheduleMeeting(c *gin.Context) {
	var req chat.UpdateScheduleMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.scheduleOrErr(c) {
		return
	}
	in := rtc.UpdateScheduleInput{
		UserID: req.UserID, OccurrenceID: req.OccurrenceID, Scope: req.Scope,
		Title: req.Title, StartTime: req.StartTime, DurationMinutes: req.DurationMinutes,
		Timezone: req.Timezone, EnableWaitingRoom: req.EnableWaitingRoom, MuteOnJoin: req.MuteOnJoin,
		AllowJoinBeforeMinutes: req.AllowJoinBeforeMinutes, RemindBeforeMinutes: req.RemindBeforeMinutes,
		Password: req.Password, ClearPassword: req.ClearPassword,
	}
	if req.Invitees != nil {
		inv := toInvitees(*req.Invitees)
		in.Invitees = &inv
	}
	if req.Recurrence != nil {
		rec := toRecurrence(*req.Recurrence)
		in.Recurrence = &rec
	}
	series, occ, err := o.ScheduleService.Update(c, in)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, scheduleDetailResp(series, occ))
}

func (o *Api) CancelScheduleMeeting(c *gin.Context) {
	var req chat.CancelScheduleMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.scheduleOrErr(c) {
		return
	}
	n, err := o.ScheduleService.Cancel(c, req.UserID, req.OccurrenceID, req.Scope)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &chat.CancelScheduleMeetingResp{Ok: true, CancelledCount: n})
}

func (o *Api) ListScheduleMeeting(c *gin.Context) {
	var req chat.ListScheduleMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.scheduleOrErr(c) {
		return
	}
	total, items, err := o.ScheduleService.List(c, req.UserID, req.Role, req.FromTime, req.ToTime, req.PageNumber, req.ShowNumber)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	list := make([]chat.ScheduleMeetingListItem, 0, len(items))
	for _, it := range items {
		list = append(list, chat.ScheduleMeetingListItem{
			MeetingID: it.Series.MeetingID, OccurrenceID: it.Occurrence.ID, SeriesID: it.Series.ID,
			Title: it.Series.Title, StartTime: it.Occurrence.StartTime, EndTime: it.Occurrence.EndTime,
			Timezone: it.Series.Timezone, Status: it.Occurrence.Status, IsHost: it.IsHost,
			RecurrenceFreq: it.Series.Recurrence.Freq, HasPassword: it.Series.PasswordHash != "",
			CallType: it.Series.CallType,
		})
	}
	apiresp.GinSuccess(c, &chat.ListScheduleMeetingResp{Total: total, List: list})
}

func (o *Api) GetScheduleMeeting(c *gin.Context) {
	var req chat.GetScheduleMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.scheduleOrErr(c) {
		return
	}
	series, occ, err := o.ScheduleService.GetAuthorized(c, req.UserID, req.MeetingID, req.OccurrenceID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, scheduleDetailResp(series, occ))
}

func scheduleDetailResp(series *chatdb.ScheduleMeetingSeries, occ *chatdb.ScheduleMeetingOccurrence) *chat.GetScheduleMeetingResp {
	if series == nil {
		return &chat.GetScheduleMeetingResp{}
	}
	resp := &chat.GetScheduleMeetingResp{
		MeetingID: series.MeetingID, SeriesID: series.ID, Title: series.Title, CallType: series.CallType,
		Timezone: series.Timezone, DurationMinutes: series.DurationMinutes,
		EnableWaitingRoom: series.EnableWaitingRoom, MuteOnJoin: series.MuteOnJoin,
		AllowJoinBeforeMinutes: series.AllowJoinBeforeMinutes, RemindBeforeMinutes: series.RemindBeforeMinutes,
		InviteeUserIDs: series.InviteeUserIDs, HasPassword: series.PasswordHash != "",
		HostUserID: series.HostUserID, RecurrenceFreq: series.Recurrence.Freq,
	}
	if occ != nil {
		resp.OccurrenceID = occ.ID
		resp.StartTime = occ.StartTime
		resp.EndTime = occ.EndTime
		resp.Status = occ.Status
	}
	return resp
}

func (o *Api) JoinScheduleMeeting(c *gin.Context) {
	var req chat.JoinScheduleMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.scheduleOrErr(c) {
		return
	}
	res, occ, err := o.ScheduleService.Join(c, req.UserID, req.MeetingID, req.OccurrenceID, req.Password)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp := &chat.JoinScheduleMeetingResp{
		Status: res.Status, OccurrenceID: occ.ID, MeetingID: occ.MeetingID, WaitingCount: res.WaitingCount,
	}
	if res.Status == "ongoing" && res.Room != nil {
		resp.RoomID = res.Room.RoomID
		resp.Token = res.Token
		resp.LiveURL = o.ConferenceManager.GetLiveKitURL()
		resp.Title = res.Room.Title
		resp.CallType = res.Room.CallType
		if res.Member != nil {
			resp.Role = res.Member.Role
		}
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) StartScheduleMeeting(c *gin.Context) {
	var req chat.StartScheduleMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.scheduleOrErr(c) {
		return
	}
	res, err := o.ScheduleService.Start(c, req.UserID, req.OccurrenceID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp := &chat.JoinScheduleMeetingResp{Status: res.Status}
	if res.Room != nil {
		resp.RoomID = res.Room.RoomID
		resp.MeetingID = res.Room.RoomID
		resp.Token = res.Token
		resp.LiveURL = o.ConferenceManager.GetLiveKitURL()
		resp.Title = res.Room.Title
		resp.CallType = res.Room.CallType
		resp.OccurrenceID = res.Room.ScheduleOccurrenceID
		if res.Member != nil {
			resp.Role = res.Member.Role
		}
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) EndScheduleMeeting(c *gin.Context) {
	var req chat.EndScheduleMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.scheduleOrErr(c) {
		return
	}
	occ, err := o.ScheduleService.End(c, req.UserID, req.MeetingID, req.OccurrenceID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &chat.EndScheduleMeetingResp{
		Ok: true, MeetingID: occ.MeetingID, OccurrenceID: occ.ID, Status: occ.Status, EndedAt: occ.EndedAt,
	})
}
