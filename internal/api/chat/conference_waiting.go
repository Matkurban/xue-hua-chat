package chat

import (
	"chat/pkg/common/rtc"
	"chat/pkg/protocol/chat"
	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"github.com/gin-gonic/gin"
)

func (o *Api) requireScheduleHost(c *gin.Context, userID, meetingID string) bool {
	series, _, err := o.ScheduleService.Get(c, meetingID, "")
	if err != nil {
		// fallback: active conference admin
		if o.ConferenceManager != nil {
			if room, ok := o.ConferenceManager.GetConference(meetingID); ok && (room.AdminUserID == userID || room.CreatorUserID == userID) {
				return true
			}
		}
		apiresp.GinError(c, err)
		return false
	}
	if series.HostUserID != userID {
		apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("only host can manage waiting room"))
		return false
	}
	return true
}

func (o *Api) ListConferenceWaiting(c *gin.Context) {
	var req chat.WaitingListReq
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
	if !o.requireScheduleHost(c, req.UserID, req.MeetingID) {
		return
	}
	list, err := o.ScheduleService.Waiting().List(c, req.MeetingID)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	out := make([]chat.WaitingUserItem, 0, len(list))
	for _, u := range list {
		out = append(out, chat.WaitingUserItem{
			UserID: u.UserID, Nickname: u.Nickname, FaceURL: u.FaceURL, EnqueueAt: u.EnqueueAt,
		})
	}
	apiresp.GinSuccess(c, &chat.WaitingListResp{List: out})
}

func (o *Api) AdmitConferenceWaiting(c *gin.Context) {
	var req chat.WaitingAdmitReq
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
	if !o.requireScheduleHost(c, req.UserID, req.MeetingID) {
		return
	}
	if err := o.ScheduleService.Waiting().Admit(c, req.MeetingID, req.TargetUserIDs); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	o.ScheduleService.Waiting().NotifyHostWaiting(c, req.UserID, req.MeetingID, "waiting_admitted", req.TargetUserIDs)
	apiresp.GinSuccess(c, map[string]any{"ok": true})
}

func (o *Api) RejectConferenceWaiting(c *gin.Context) {
	var req chat.WaitingRejectReq
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
	if !o.requireScheduleHost(c, req.UserID, req.MeetingID) {
		return
	}
	var store *rtc.RedisConferenceStore
	if o.ConferenceManager != nil {
		store = o.ConferenceManager.ConferenceStore()
	}
	if err := o.ScheduleService.Waiting().Reject(c, store, req.MeetingID, req.TargetUserIDs); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	o.ScheduleService.Waiting().NotifyHostWaiting(c, req.UserID, req.MeetingID, "waiting_rejected", req.TargetUserIDs)
	apiresp.GinSuccess(c, map[string]any{"ok": true})
}

func (o *Api) AdmitAllConferenceWaiting(c *gin.Context) {
	var req chat.WaitingAdmitAllReq
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
	if !o.requireScheduleHost(c, req.UserID, req.MeetingID) {
		return
	}
	list, err := o.ScheduleService.Waiting().List(c, req.MeetingID)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	ids := make([]string, 0, len(list))
	for _, u := range list {
		ids = append(ids, u.UserID)
	}
	if err := o.ScheduleService.Waiting().Admit(c, req.MeetingID, ids); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	o.ScheduleService.Waiting().NotifyHostWaiting(c, req.UserID, req.MeetingID, "waiting_admitted", ids)
	apiresp.GinSuccess(c, map[string]any{"ok": true, "count": len(ids)})
}

func (o *Api) LeaveConferenceWaiting(c *gin.Context) {
	var req chat.LeaveWaitingReq
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
	_ = o.ScheduleService.Waiting().LeaveWaiting(c, req.MeetingID, req.UserID)
	apiresp.GinSuccess(c, map[string]any{"ok": true})
}
