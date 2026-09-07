package chat

import "gitee.com/matkurban/openimsdk-tools/errs"

// ---------------------------------------------------------------------------
// Meeting / Call Management
// ---------------------------------------------------------------------------

// MeetingParticipantInfo represents a participant in a meeting.
type MeetingParticipantInfo struct {
	UserID   string `json:"userID"`
	JoinTime int64  `json:"joinTime"`
}

// CreateMeetingReq request to create a new meeting room.
type CreateMeetingReq struct {
	CreatorUserID  string   `json:"creatorUserID"`
	CallType       string   `json:"callType"` // "audio" or "video"
	InviteeUserIDs []string `json:"inviteeUserIDs"`
}

func (x *CreateMeetingReq) Check() error {
	if x.CreatorUserID == "" {
		return errs.ErrArgs.WrapMsg("creatorUserID is empty")
	}
	if x.CallType != "audio" && x.CallType != "video" {
		return errs.ErrArgs.WrapMsg("callType must be 'audio' or 'video'")
	}
	if len(x.InviteeUserIDs) == 0 {
		return errs.ErrArgs.WrapMsg("inviteeUserIDs cannot be empty")
	}
	if len(x.InviteeUserIDs) > 8 {
		return errs.ErrArgs.WrapMsg("inviteeUserIDs cannot exceed 8 (total 9 max)")
	}
	return nil
}

// CreateMeetingResp response for creating a meeting room.
type CreateMeetingResp struct {
	RoomID    string   `json:"roomID"`
	Token     string   `json:"token"`
	LiveURL   string   `json:"liveURL"`
	BusyUsers []string `json:"busyUsers,omitempty"`
}

// JoinMeetingReq request to join an existing meeting room.
type JoinMeetingReq struct {
	RoomID string `json:"roomID"`
	UserID string `json:"userID"`
}

func (x *JoinMeetingReq) Check() error {
	if x.RoomID == "" {
		return errs.ErrArgs.WrapMsg("roomID is empty")
	}
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID is empty")
	}
	return nil
}

// JoinMeetingResp response for joining a meeting room.
type JoinMeetingResp struct {
	Token        string                    `json:"token"`
	LiveURL      string                    `json:"liveURL"`
	RoomID       string                    `json:"roomID"`
	CallType     string                    `json:"callType"`
	Participants []*MeetingParticipantInfo `json:"participants"`
}

// LeaveMeetingReq request to leave a meeting room.
type LeaveMeetingReq struct {
	RoomID string `json:"roomID"`
	UserID string `json:"userID"`
}

func (x *LeaveMeetingReq) Check() error {
	if x.RoomID == "" {
		return errs.ErrArgs.WrapMsg("roomID is empty")
	}
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID is empty")
	}
	return nil
}

// LeaveMeetingResp response for leaving a meeting room.
type LeaveMeetingResp struct{}

// EndMeetingReq request to end a meeting.
type EndMeetingReq struct {
	RoomID string `json:"roomID"`
	UserID string `json:"userID"`
}

func (x *EndMeetingReq) Check() error {
	if x.RoomID == "" {
		return errs.ErrArgs.WrapMsg("roomID is empty")
	}
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID is empty")
	}
	return nil
}

// EndMeetingResp response for ending a meeting.
type EndMeetingResp struct {
	ParticipantUserIDs []string `json:"participantUserIDs,omitempty"`
}

// GetMeetingReq request to get meeting info.
type GetMeetingReq struct {
	RoomID string `json:"roomID"`
}

func (x *GetMeetingReq) Check() error {
	if x.RoomID == "" {
		return errs.ErrArgs.WrapMsg("roomID is empty")
	}
	return nil
}

// GetMeetingResp response for getting meeting info.
type GetMeetingResp struct {
	RoomID         string                    `json:"roomID"`
	CreatorUserID  string                    `json:"creatorUserID"`
	CallType       string                    `json:"callType"`
	InviteeUserIDs []string                  `json:"inviteeUserIDs"`
	Participants   []*MeetingParticipantInfo `json:"participants"`
	CreateTime     int64                     `json:"createTime"`
}

// GetUserActiveMeetingReq request to get user's active meeting.
type GetUserActiveMeetingReq struct {
	UserID string `json:"userID"`
}

func (x *GetUserActiveMeetingReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID is empty")
	}
	return nil
}

// GetUserActiveMeetingResp response for getting user's active meeting.
type GetUserActiveMeetingResp struct {
	HasActive bool            `json:"hasActive"`
	Meeting   *GetMeetingResp `json:"meeting,omitempty"`
}
