package chat

import (
	"chat/pkg/protocol/chat"
	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/errs"

	"github.com/gin-gonic/gin"
)

// CreateMeeting creates a new meeting room and returns the room info + creator token.
func (o *Api) CreateMeeting(c *gin.Context) {
	var req chat.CreateMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request body: "+err.Error()))
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.MeetingManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("MeetingManager is not configured"))
		return
	}

	// Check which invitees are already in a call
	busyUsers := o.MeetingManager.GetBusyUsers(req.InviteeUserIDs)

	room, token, err := o.MeetingManager.CreateMeeting(req.CreatorUserID, req.CallType, req.InviteeUserIDs)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}

	apiresp.GinSuccess(c, &chat.CreateMeetingResp{
		RoomID:    room.RoomID,
		Token:     token,
		LiveURL:   o.MeetingManager.GetLiveKitURL(),
		BusyUsers: busyUsers,
	})
}

// JoinMeeting joins a user to an existing meeting and returns their token.
func (o *Api) JoinMeeting(c *gin.Context) {
	var req chat.JoinMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request body: "+err.Error()))
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.MeetingManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("MeetingManager is not configured"))
		return
	}

	token, room, err := o.MeetingManager.JoinMeeting(req.RoomID, req.UserID)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}

	participants := make([]*chat.MeetingParticipantInfo, 0, len(room.Participants))
	for _, p := range room.Participants {
		participants = append(participants, &chat.MeetingParticipantInfo{
			UserID:   p.UserID,
			JoinTime: p.JoinTime,
		})
	}

	apiresp.GinSuccess(c, &chat.JoinMeetingResp{
		Token:        token,
		LiveURL:      o.MeetingManager.GetLiveKitURL(),
		RoomID:       room.RoomID,
		CallType:     room.CallType,
		Participants: participants,
	})
}

// LeaveMeeting removes a user from a meeting.
func (o *Api) LeaveMeeting(c *gin.Context) {
	var req chat.LeaveMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request body: "+err.Error()))
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.MeetingManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("MeetingManager is not configured"))
		return
	}

	if err := o.MeetingManager.LeaveMeeting(req.RoomID, req.UserID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}

	apiresp.GinSuccess(c, &chat.LeaveMeetingResp{})
}

// EndMeeting ends a meeting and notifies all participants.
func (o *Api) EndMeeting(c *gin.Context) {
	var req chat.EndMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request body: "+err.Error()))
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.MeetingManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("MeetingManager is not configured"))
		return
	}

	participantIDs, err := o.MeetingManager.EndMeeting(req.RoomID, req.UserID)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}

	apiresp.GinSuccess(c, &chat.EndMeetingResp{
		ParticipantUserIDs: participantIDs,
	})
}

// GetMeeting returns the current state of a meeting room.
func (o *Api) GetMeeting(c *gin.Context) {
	var req chat.GetMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request body: "+err.Error()))
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.MeetingManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("MeetingManager is not configured"))
		return
	}

	room, ok := o.MeetingManager.GetMeeting(req.RoomID)
	if !ok {
		apiresp.GinError(c, errs.ErrRecordNotFound.WrapMsg("room not found"))
		return
	}

	participants := make([]*chat.MeetingParticipantInfo, 0, len(room.Participants))
	for _, p := range room.Participants {
		participants = append(participants, &chat.MeetingParticipantInfo{
			UserID:   p.UserID,
			JoinTime: p.JoinTime,
		})
	}

	apiresp.GinSuccess(c, &chat.GetMeetingResp{
		RoomID:         room.RoomID,
		CreatorUserID:  room.CreatorUserID,
		CallType:       room.CallType,
		InviteeUserIDs: room.InviteeUserIDs,
		Participants:   participants,
		CreateTime:     room.CreateTime,
	})
}

// GetUserActiveMeeting returns the user's current active meeting if any.
func (o *Api) GetUserActiveMeeting(c *gin.Context) {
	var req chat.GetUserActiveMeetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid request body: "+err.Error()))
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if o.MeetingManager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("MeetingManager is not configured"))
		return
	}

	room, ok := o.MeetingManager.GetUserActiveMeeting(req.UserID)
	if !ok {
		apiresp.GinSuccess(c, &chat.GetUserActiveMeetingResp{HasActive: false})
		return
	}

	participants := make([]*chat.MeetingParticipantInfo, 0, len(room.Participants))
	for _, p := range room.Participants {
		participants = append(participants, &chat.MeetingParticipantInfo{
			UserID:   p.UserID,
			JoinTime: p.JoinTime,
		})
	}

	apiresp.GinSuccess(c, &chat.GetUserActiveMeetingResp{
		HasActive: true,
		Meeting: &chat.GetMeetingResp{
			RoomID:         room.RoomID,
			CreatorUserID:  room.CreatorUserID,
			CallType:       room.CallType,
			InviteeUserIDs: room.InviteeUserIDs,
			Participants:   participants,
			CreateTime:     room.CreateTime,
		},
	})
}
