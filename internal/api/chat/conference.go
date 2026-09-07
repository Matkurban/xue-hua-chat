package chat

import (
	"strings"

	"chat/pkg/common/rtc"
	"chat/pkg/protocol/chat"
	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"github.com/gin-gonic/gin"
)

func (o *Api) confMgrOrErr(c *gin.Context) bool {
	if o.ConferenceManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("ConferenceManager is not configured"))
		return false
	}
	return true
}

func (o *Api) CreateConference(c *gin.Context) {
	var req chat.CreateConferenceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if !bindConferenceOpUser(c, &req.CreatorUserID) {
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := rtc.ValidateMeetingPasswordFormat(req.Password); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	pwdHash, err := rtc.HashMeetingPassword(req.Password)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	room, token, err := o.ConferenceManager.CreateConferenceOpts(rtc.CreateConferenceOpts{
		CreatorUserID:  req.CreatorUserID,
		CallType:       req.CallType,
		Title:          req.Title,
		InviteeUserIDs: req.InviteeUserIDs,
		GroupID:        req.GroupID,
		ConversationID: req.ConversationID,
		PasswordHash:   pwdHash,
	})
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	creator := &rtc.ConferenceMember{
		UserID: req.CreatorUserID, Role: rtc.ConferenceRoleAdmin, OnStage: true,
	}
	if mem, ok := o.ConferenceManager.GetMember(room.RoomID, req.CreatorUserID); ok && mem != nil {
		creator = mem
	}
	apiresp.GinSuccess(c, &chat.CreateConferenceResp{
		RoomID: room.RoomID, Token: token, LiveURL: o.ConferenceManager.GetLiveKitURL(),
		Title: room.Title, CallType: room.CallType, Role: creator.Role,
		OnStage: creator.OnStage, CameraOn: creator.CameraOn,
		CameraOnUserIDs: append([]string(nil), room.CameraOnUserIDs...),
		CameraOnCount:   len(room.CameraOnUserIDs),
		AutoStageLimit:  room.AutoStageLimit,
		OnStageCount:    o.ConferenceManager.CountOnStage(room.RoomID),
		CanOpenCamera:   canOpenCameraResp(room, creator),
		LinkMicEnabled:  !room.LinkMicDisabled,
	})
}

func (o *Api) JoinConference(c *gin.Context) {
	var req chat.JoinConferenceReq
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
	if !o.confMgrOrErr(c) {
		return
	}
	res, err := o.ConferenceManager.JoinConferenceWithOpts(req.RoomID, req.UserID, req.Password, false)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "password") || strings.Contains(msg, "waiting") || strings.Contains(msg, "locked") {
			apiresp.GinError(c, errs.ErrArgs.WrapMsg(msg))
			return
		}
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(msg))
		return
	}
	if res.Status == "waiting" {
		apiresp.GinSuccess(c, &chat.JoinScheduleMeetingResp{
			Status: "waiting", MeetingID: req.RoomID, RoomID: req.RoomID, WaitingCount: res.WaitingCount,
		})
		return
	}
	room, member := res.Room, res.Member
	apiresp.GinSuccess(c, &chat.JoinConferenceResp{
		Token: res.Token, LiveURL: o.ConferenceManager.GetLiveKitURL(),
		RoomID: room.RoomID, CallType: room.CallType, Title: room.Title, Role: member.Role,
		GlobalAudioMuted: room.GlobalAudioMuted, GlobalVideoOff: room.GlobalVideoOff,
		AudioForcedOff: member.AudioForcedOff, VideoForcedOff: member.VideoForcedOff,
		ChatForcedOff: member.ChatForcedOff,
		OnStage:       member.OnStage, SpeakPending: member.SpeakPending, CameraOn: member.CameraOn,
		CameraOnUserIDs: append([]string(nil), room.CameraOnUserIDs...),
		CameraOnCount:   len(room.CameraOnUserIDs),
		AutoStageLimit:  room.AutoStageLimit,
		OnStageCount:    o.ConferenceManager.CountOnStage(room.RoomID),
		CanOpenCamera:   canOpenCameraResp(room, member),
		LinkMicEnabled:  !room.LinkMicDisabled,
	})
}

func canOpenCameraResp(room *rtc.ConferenceRoom, mem *rtc.ConferenceMember) bool {
	return rtc.CanOpenCamera(room, mem)
}

func (o *Api) LeaveConference(c *gin.Context) {
	var req chat.ConferenceRoomIDUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	ended, err := o.ConferenceManager.LeaveConference(req.RoomID, req.UserID)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{"ended": ended})
}

func (o *Api) ShareConference(c *gin.Context) {
	var req chat.ShareConferenceReq
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
	if !o.confMgrOrErr(c) {
		return
	}
	shared, err := o.ConferenceManager.ShareConference(req.RoomID, req.UserID, req.ConversationIDs)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{"shared": shared})
}

func (o *Api) RegisterConferenceAnnounce(c *gin.Context) {
	var req chat.RegisterConferenceAnnounceReq
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
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.RegisterConferenceAnnounce(
		req.RoomID, req.UserID, req.ConversationID, req.ClientMsgID, req.ServerMsgID,
	); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) EndConference(c *gin.Context) {
	var req chat.ConferenceRoomIDUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.EndConference(req.RoomID, req.UserID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) GetConference(c *gin.Context) {
	var req struct {
		RoomID string `json:"roomID"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("roomID required"))
		return
	}
	if err := chat.CheckConferenceRoomID(req.RoomID); err != nil {
		apiresp.GinError(c, err)
		return
	}
	opUserID := ""
	if !bindConferenceOpUser(c, &opUserID) {
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	room, ok := o.ConferenceManager.GetConference(req.RoomID)
	if !ok {
		apiresp.GinError(c, errs.ErrRecordNotFound.WrapMsg("conference not found"))
		return
	}
	hasPassword := room.PasswordHash != ""
	mem, isMember := o.ConferenceManager.GetMember(req.RoomID, opUserID)
	if !isMember {
		// Pre-join probe: public join metadata only (incl. hasPassword).
		apiresp.GinSuccess(c, &chat.GetConferenceResp{
			RoomID: room.RoomID, CallType: room.CallType, Title: room.Title,
			Locked: room.Locked, HasPassword: hasPassword,
			LiveURL: o.ConferenceManager.GetLiveKitURL(),
		})
		return
	}
	resp := &chat.GetConferenceResp{
		RoomID: room.RoomID, AdminUserID: room.AdminUserID, CallType: room.CallType, Title: room.Title,
		ParticipantCount: room.ParticipantCount, Rev: room.Rev,
		GlobalAudioMuted: room.GlobalAudioMuted, GlobalVideoOff: room.GlobalVideoOff,
		ChatMuted: room.ChatMuted, Locked: room.Locked, PinnedUserID: room.PinnedUserID,
		EgressID: room.EgressID, IngressID: room.IngressID,
		LiveURL: o.ConferenceManager.GetLiveKitURL(),
		Role:    mem.Role, AudioForcedOff: mem.AudioForcedOff, VideoForcedOff: mem.VideoForcedOff,
		ChatForcedOff:  mem.ChatForcedOff,
		GroupID:        room.GroupID,
		HasPassword:    hasPassword,
		LinkMicEnabled: !room.LinkMicDisabled,
	}
	if opUserID == room.AdminUserID {
		resp.IngressURL = room.IngressURL
		resp.IngressStreamKey = room.IngressStreamKey
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) ListActiveConferences(c *gin.Context) {
	if !o.confMgrOrErr(c) {
		return
	}
	var req chat.ListActiveConferencesReq
	_ = c.ShouldBindJSON(&req)
	pageNumber, showNumber := req.NormalizePage()
	total, list, err := o.ConferenceManager.ListActiveConferences(c.Request.Context(), pageNumber, showNumber)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	items := make([]*chat.ActiveConferenceItem, 0, len(list))
	for _, s := range list {
		pub := rtc.PublicActiveConferenceFromSummary(s)
		if pub == nil {
			continue
		}
		items = append(items, &chat.ActiveConferenceItem{
			RoomID:           pub.RoomID,
			Title:            pub.Title,
			CallType:         pub.CallType,
			CreatorUserID:    pub.CreatorUserID,
			AdminUserID:      pub.AdminUserID,
			ParticipantCount: pub.ParticipantCount,
			CreateTime:       pub.CreateTime,
			Locked:           pub.Locked,
			HasPassword:      pub.HasPassword,
		})
	}
	apiresp.GinSuccess(c, &chat.ListActiveConferencesResp{
		Total:       total,
		Conferences: items,
	})
}

func (o *Api) ListConferenceParticipants(c *gin.Context) {
	var req chat.ConferenceParticipantsReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	opUserID := ""
	if !bindConferenceOpUser(c, &opUserID) {
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if !o.ConferenceManager.IsMember(req.RoomID, opUserID) {
		apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("not in conference"))
		return
	}
	page, err := o.ConferenceManager.ListParticipants(req.RoomID, req.PageNumber, req.PageSize, req.Role, req.Keyword)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	list := make([]*chat.ConferenceParticipantItem, 0, len(page.List))
	for _, m := range page.List {
		list = append(list, &chat.ConferenceParticipantItem{
			UserID: m.UserID, Role: m.Role, JoinTime: m.JoinTime,
			AudioForcedOff: m.AudioForcedOff,
			VideoForcedOff: m.VideoForcedOff, ChatForcedOff: m.ChatForcedOff,
			InLiveKit: m.InLiveKit,
			OnStage:   m.OnStage, CameraOn: m.CameraOn, SpeakPending: m.SpeakPending,
			Nickname: m.Nickname, FaceURL: m.FaceURL,
		})
	}
	apiresp.GinSuccess(c, &chat.ConferenceParticipantsResp{
		Total: page.Total, PageNumber: page.PageNumber, PageSize: page.PageSize, List: list,
	})
}

func (o *Api) InviteConference(c *gin.Context) {
	var req chat.ConferenceInviteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.Invite(req.RoomID, req.UserID, req.InviteeUserIDs); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) TransferConferenceAdmin(c *gin.Context) {
	var req chat.ConferenceTransferAdminReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.TransferAdmin(req.RoomID, req.UserID, req.TargetUserID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) SetConferenceCohost(c *gin.Context) {
	var req chat.ConferenceSetCohostReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.SetCohost(req.RoomID, req.UserID, req.TargetUserID, req.Cohost); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) KickConference(c *gin.Context) {
	var req chat.ConferenceKickReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.Kick(req.RoomID, req.UserID, req.TargetUserID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) RemoveConference(c *gin.Context) {
	var req chat.ConferenceRemoveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.Remove(req.RoomID, req.UserID, req.TargetUserID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) ApplyConferenceSpeak(c *gin.Context) {
	var req chat.ConferenceRoomIDUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.ApplySpeak(req.RoomID, req.UserID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) ApproveConferenceSpeak(c *gin.Context) {
	var req chat.ConferenceSpeakApproveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.ApproveSpeak(req.RoomID, req.UserID, req.TargetUserID, req.Accept); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) LeaveConferenceSpeak(c *gin.Context) {
	var req chat.ConferenceSpeakLeaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.LeaveSpeak(req.RoomID, req.UserID, req.TargetUserID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) SetConferenceLinkMic(c *gin.Context) {
	var req chat.ConferenceBoolFlagReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.SetLinkMic(req.RoomID, req.UserID, req.Value); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) ToggleConferenceCamera(c *gin.Context) {
	var req chat.ConferenceCameraToggleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.ToggleCamera(req.RoomID, req.UserID, req.Enabled); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) SetConferenceMedia(c *gin.Context) {
	var req chat.ConferenceMediaReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.SetMedia(req.RoomID, req.UserID, req.TargetUserID, req.Audio, req.Video); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) SetConferenceMediaAll(c *gin.Context) {
	var req chat.ConferenceMediaAllReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.SetMediaAll(req.RoomID, req.UserID, req.Audio, req.Video); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) LockConference(c *gin.Context) {
	var req chat.ConferenceBoolFlagReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.SetLocked(req.RoomID, req.UserID, req.Value); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) ChatMuteConference(c *gin.Context) {
	var req chat.ConferenceBoolFlagReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.SetChatMuted(req.RoomID, req.UserID, req.Value); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) MemberChatMuteConference(c *gin.Context) {
	var req chat.ConferenceMemberChatMuteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.SetMemberChatMuted(req.RoomID, req.UserID, req.TargetUserID, req.Value); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) RevokeConferenceChat(c *gin.Context) {
	var req chat.ConferenceRevokeChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.RevokeChat(req.RoomID, req.UserID, req.TargetUserID, req.Seq); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) PinConference(c *gin.Context) {
	var req chat.ConferencePinReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.Pin(req.RoomID, req.UserID, req.TargetUserID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) StartConferenceEgress(c *gin.Context) {
	var req chat.ConferenceEgressStartReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	id, err := o.ConferenceManager.StartEgress(req.RoomID, req.UserID, req.Filepath)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, &chat.ConferenceEgressStartResp{EgressID: id})
}

func (o *Api) StopConferenceEgress(c *gin.Context) {
	var req chat.ConferenceRoomIDUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.StopEgress(req.RoomID, req.UserID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}

func (o *Api) CreateConferenceIngress(c *gin.Context) {
	var req chat.ConferenceIngressCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	id, url, key, err := o.ConferenceManager.CreateIngress(req.RoomID, req.UserID, req.Name, req.InputType)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, &chat.ConferenceIngressCreateResp{IngressID: id, URL: url, StreamKey: key})
}

func (o *Api) DeleteConferenceIngress(c *gin.Context) {
	var req chat.ConferenceRoomIDUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !bindConferenceOpUser(c, &req.UserID) {
		return
	}
	if req.Check() != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request"))
		return
	}
	if !o.confMgrOrErr(c) {
		return
	}
	if err := o.ConferenceManager.DeleteIngress(req.RoomID, req.UserID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, map[string]any{})
}
