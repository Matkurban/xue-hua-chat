package chat

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"chat/pkg/common/errs"
)

const ConferenceTitleMaxRunes = 40

var conferenceRoomIDRe = regexp.MustCompile(`^[0-9]{9}$`)

// CheckConferenceRoomID requires a fixed-length 9-digit room ID.
func CheckConferenceRoomID(roomID string) error {
	if !conferenceRoomIDRe.MatchString(roomID) {
		return errs.ErrArgs.WrapMsg("roomID must be a 9-digit number")
	}
	return nil
}

// NormalizeConferenceTitle trims and validates a required conference title.
func NormalizeConferenceTitle(title string) (string, error) {
	t := strings.TrimSpace(title)
	if t == "" {
		return "", errs.ErrArgs.WrapMsg("title is required")
	}
	if utf8.RuneCountInString(t) > ConferenceTitleMaxRunes {
		return "", errs.ErrArgs.WrapMsg("title must be at most 40 characters")
	}
	return t, nil
}

type CreateConferenceReq struct {
	CreatorUserID  string   `json:"creatorUserID"`
	CallType       string   `json:"callType"`
	Title          string   `json:"title"`
	InviteeUserIDs []string `json:"inviteeUserIDs"`
	GroupID        string   `json:"groupID,omitempty"`
	ConversationID string   `json:"conversationID,omitempty"` // source chat for announce card
	Password       string   `json:"password,omitempty"`       // optional; empty = none
}

func (x *CreateConferenceReq) Check() error {
	if x.CreatorUserID == "" {
		return errs.ErrArgs.WrapMsg("creatorUserID is empty")
	}
	if x.CallType != "audio" && x.CallType != "video" {
		return errs.ErrArgs.WrapMsg("callType must be audio or video")
	}
	title, err := NormalizeConferenceTitle(x.Title)
	if err != nil {
		return err
	}
	x.Title = title
	return nil
}

type CreateConferenceResp struct {
	RoomID          string   `json:"roomID"`
	Token           string   `json:"token"`
	LiveURL         string   `json:"liveURL"`
	Title           string   `json:"title,omitempty"`
	CallType        string   `json:"callType"`
	Role            string   `json:"role"`
	CameraOnUserIDs []string `json:"cameraOnUserIDs,omitempty"`
	CameraOnCount   int      `json:"cameraOnCount"`
	AutoStageLimit  int      `json:"autoStageLimit"`
	OnStageCount    int      `json:"onStageCount"`
	OnStage         bool     `json:"onStage"`
	CameraOn        bool     `json:"cameraOn,omitempty"`
	CanOpenCamera   bool     `json:"canOpenCamera"`
	LinkMicEnabled  bool     `json:"linkMicEnabled"`
}

type JoinConferenceReq struct {
	RoomID   string `json:"roomID"`
	UserID   string `json:"userID"`
	Password string `json:"password,omitempty"`
}

type JoinConferenceWaitingResp struct {
	Status       string `json:"status"` // ongoing | waiting
	WaitingCount int64  `json:"waitingCount,omitempty"`
}

func (x *JoinConferenceReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type JoinConferenceResp struct {
	Token            string   `json:"token"`
	LiveURL          string   `json:"liveURL"`
	RoomID           string   `json:"roomID"`
	CallType         string   `json:"callType"`
	Title            string   `json:"title,omitempty"`
	Role             string   `json:"role"`
	GlobalAudioMuted bool     `json:"globalAudioMuted,omitempty"`
	GlobalVideoOff   bool     `json:"globalVideoOff,omitempty"`
	AudioForcedOff   bool     `json:"audioForcedOff,omitempty"`
	VideoForcedOff   bool     `json:"videoForcedOff,omitempty"`
	ChatForcedOff    bool     `json:"chatForcedOff,omitempty"`
	OnStage          bool     `json:"onStage"`
	SpeakPending     bool     `json:"speakPending,omitempty"`
	CameraOn         bool     `json:"cameraOn,omitempty"`
	CameraOnUserIDs  []string `json:"cameraOnUserIDs,omitempty"`
	CameraOnCount    int      `json:"cameraOnCount"`
	AutoStageLimit   int      `json:"autoStageLimit"`
	OnStageCount     int      `json:"onStageCount"`
	CanOpenCamera    bool     `json:"canOpenCamera"`
	LinkMicEnabled   bool     `json:"linkMicEnabled"`
}

type ConferenceRoomIDUserReq struct {
	RoomID string `json:"roomID"`
	UserID string `json:"userID"`
}

func (x *ConferenceRoomIDUserReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type GetConferenceResp struct {
	RoomID           string `json:"roomID"`
	AdminUserID      string `json:"adminUserID"`
	CallType         string `json:"callType"`
	Title            string `json:"title,omitempty"`
	PinnedUserID     string `json:"pinnedUserID,omitempty"`
	EgressID         string `json:"egressID,omitempty"`
	IngressID        string `json:"ingressID,omitempty"`
	IngressURL       string `json:"ingressURL,omitempty"`
	IngressStreamKey string `json:"ingressStreamKey,omitempty"`
	LiveURL          string `json:"liveURL"`
	Role             string `json:"role,omitempty"`
	GroupID          string `json:"groupID,omitempty"`
	ParticipantCount int    `json:"participantCount"`
	Rev              int64  `json:"rev"`
	GlobalAudioMuted bool   `json:"globalAudioMuted"`
	GlobalVideoOff   bool   `json:"globalVideoOff"`
	ChatMuted        bool   `json:"chatMuted"`
	Locked           bool   `json:"locked"`
	AudioForcedOff   bool   `json:"audioForcedOff,omitempty"`
	VideoForcedOff   bool   `json:"videoForcedOff,omitempty"`
	ChatForcedOff    bool   `json:"chatForcedOff,omitempty"`
	HasPassword      bool   `json:"hasPassword,omitempty"`
	LinkMicEnabled   bool   `json:"linkMicEnabled"`
}

type ConferenceParticipantsReq struct {
	RoomID     string `json:"roomID"`
	PageNumber int32  `json:"pageNumber"`
	PageSize   int32  `json:"pageSize"`
	Role       string `json:"role,omitempty"`
	Keyword    string `json:"keyword,omitempty"`
}

func (x *ConferenceParticipantsReq) Check() error {
	return CheckConferenceRoomID(x.RoomID)
}

type ConferenceParticipantItem struct {
	UserID         string `json:"userID"`
	Role           string `json:"role"`
	JoinTime       int64  `json:"joinTime"`
	AudioForcedOff bool   `json:"audioForcedOff"`
	VideoForcedOff bool   `json:"videoForcedOff"`
	ChatForcedOff  bool   `json:"chatForcedOff"`
	InLiveKit      bool   `json:"inLiveKit"`
	OnStage        bool   `json:"onStage"`
	CameraOn       bool   `json:"cameraOn"`
	SpeakPending   bool   `json:"speakPending,omitempty"`
	Nickname       string `json:"nickname,omitempty"`
	FaceURL        string `json:"faceURL,omitempty"`
}

type ConferenceSpeakLeaveReq struct {
	RoomID       string `json:"roomID"`
	UserID       string `json:"userID"`
	TargetUserID string `json:"targetUserID,omitempty"`
}

func (x *ConferenceSpeakLeaveReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type ConferenceSpeakApproveReq struct {
	RoomID       string `json:"roomID"`
	UserID       string `json:"userID"`
	TargetUserID string `json:"targetUserID"`
	Accept       bool   `json:"accept"`
}

func (x *ConferenceSpeakApproveReq) Check() error {
	if x.UserID == "" || x.TargetUserID == "" {
		return errs.ErrArgs.WrapMsg("userID, targetUserID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type ConferenceCameraToggleReq struct {
	RoomID  string `json:"roomID"`
	UserID  string `json:"userID"`
	Enabled bool   `json:"enabled"`
}

func (x *ConferenceCameraToggleReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type ConferenceParticipantsResp struct {
	Total      int64                        `json:"total"`
	PageNumber int32                        `json:"pageNumber"`
	PageSize   int32                        `json:"pageSize"`
	List       []*ConferenceParticipantItem `json:"list"`
}

type ConferenceInviteReq struct {
	RoomID         string   `json:"roomID"`
	UserID         string   `json:"userID"`
	InviteeUserIDs []string `json:"inviteeUserIDs"`
}

func (x *ConferenceInviteReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	if err := CheckConferenceRoomID(x.RoomID); err != nil {
		return err
	}
	if len(x.InviteeUserIDs) == 0 {
		return errs.ErrArgs.WrapMsg("inviteeUserIDs required")
	}
	return nil
}

type ConferenceTransferAdminReq struct {
	RoomID       string `json:"roomID"`
	UserID       string `json:"userID"`
	TargetUserID string `json:"targetUserID"`
}

func (x *ConferenceTransferAdminReq) Check() error {
	if x.UserID == "" || x.TargetUserID == "" {
		return errs.ErrArgs.WrapMsg("userID, targetUserID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type ConferenceKickReq = ConferenceTransferAdminReq

type ConferenceRemoveReq = ConferenceTransferAdminReq

type ConferenceSetCohostReq struct {
	RoomID       string `json:"roomID"`
	UserID       string `json:"userID"`
	TargetUserID string `json:"targetUserID"`
	Cohost       bool   `json:"cohost"`
}

func (x *ConferenceSetCohostReq) Check() error {
	if x.UserID == "" || x.TargetUserID == "" {
		return errs.ErrArgs.WrapMsg("userID, targetUserID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type ConferenceMediaReq struct {
	RoomID       string `json:"roomID"`
	UserID       string `json:"userID"`
	TargetUserID string `json:"targetUserID"`
	Audio        *bool  `json:"audio,omitempty"`
	Video        *bool  `json:"video,omitempty"`
}

func (x *ConferenceMediaReq) Check() error {
	if x.UserID == "" || x.TargetUserID == "" {
		return errs.ErrArgs.WrapMsg("userID, targetUserID required")
	}
	if err := CheckConferenceRoomID(x.RoomID); err != nil {
		return err
	}
	if x.Audio == nil && x.Video == nil {
		return errs.ErrArgs.WrapMsg("audio or video required")
	}
	return nil
}

type ConferenceMediaAllReq struct {
	RoomID string `json:"roomID"`
	UserID string `json:"userID"`
	Audio  *bool  `json:"audio,omitempty"`
	Video  *bool  `json:"video,omitempty"`
}

func (x *ConferenceMediaAllReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	if err := CheckConferenceRoomID(x.RoomID); err != nil {
		return err
	}
	if x.Audio == nil && x.Video == nil {
		return errs.ErrArgs.WrapMsg("audio or video required")
	}
	return nil
}

type ConferenceBoolFlagReq struct {
	RoomID string `json:"roomID"`
	UserID string `json:"userID"`
	Value  bool   `json:"value"`
}

func (x *ConferenceBoolFlagReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

// ConferenceMemberChatMuteReq 房主对单个成员开启/解除文字禁言。
type ConferenceMemberChatMuteReq struct {
	RoomID       string `json:"roomID"`
	UserID       string `json:"userID"`
	TargetUserID string `json:"targetUserID"`
	Value        bool   `json:"value"`
}

func (x *ConferenceMemberChatMuteReq) Check() error {
	if x.UserID == "" || x.TargetUserID == "" {
		return errs.ErrArgs.WrapMsg("userID, targetUserID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

// ConferenceRevokeChatReq 撤回会中弹幕（本人或房主）。
type ConferenceRevokeChatReq struct {
	RoomID       string `json:"roomID"`
	UserID       string `json:"userID"`
	TargetUserID string `json:"targetUserID"` // 原发送者
	Seq          uint64 `json:"seq"`
}

func (x *ConferenceRevokeChatReq) Check() error {
	if x.UserID == "" || x.TargetUserID == "" {
		return errs.ErrArgs.WrapMsg("userID, targetUserID required")
	}
	if x.Seq == 0 {
		return errs.ErrArgs.WrapMsg("seq required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type ConferencePinReq struct {
	RoomID       string `json:"roomID"`
	UserID       string `json:"userID"`
	TargetUserID string `json:"targetUserID"`
}

func (x *ConferencePinReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type ShareConferenceReq struct {
	RoomID          string   `json:"roomID"`
	UserID          string   `json:"userID"`
	ConversationIDs []string `json:"conversationIDs"`
}

func (x *ShareConferenceReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	if err := CheckConferenceRoomID(x.RoomID); err != nil {
		return err
	}
	if len(x.ConversationIDs) == 0 {
		return errs.ErrArgs.WrapMsg("conversationIDs required")
	}
	return nil
}

// RegisterConferenceAnnounceReq registers a client-sent type-127 announce card.
type RegisterConferenceAnnounceReq struct {
	RoomID         string `json:"roomID"`
	UserID         string `json:"userID"`
	ConversationID string `json:"conversationID"`
	ClientMsgID    string `json:"clientMsgID"`
	ServerMsgID    string `json:"serverMsgID,omitempty"`
}

func (x *RegisterConferenceAnnounceReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	if err := CheckConferenceRoomID(x.RoomID); err != nil {
		return err
	}
	if x.ConversationID == "" || x.ClientMsgID == "" {
		return errs.ErrArgs.WrapMsg("conversationID and clientMsgID required")
	}
	return nil
}

type ConferenceEgressStartReq struct {
	RoomID   string `json:"roomID"`
	UserID   string `json:"userID"`
	Filepath string `json:"filepath,omitempty"`
}

func (x *ConferenceEgressStartReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type ConferenceEgressStartResp struct {
	EgressID string `json:"egressID"`
}

type ConferenceIngressCreateReq struct {
	RoomID    string `json:"roomID"`
	UserID    string `json:"userID"`
	Name      string `json:"name,omitempty"`
	InputType string `json:"inputType,omitempty"` // rtmp (default) | whip
}

func (x *ConferenceIngressCreateReq) Check() error {
	if x.UserID == "" {
		return errs.ErrArgs.WrapMsg("userID required")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type ConferenceIngressCreateResp struct {
	IngressID string `json:"ingressID"`
	URL       string `json:"url"`
	StreamKey string `json:"streamKey"`
}

// ─── Client active-conference list ───────────────────────────────────────────

type ListActiveConferencesReq struct {
	PageNumber int32 `json:"pageNumber"`
	ShowNumber int32 `json:"showNumber"`
	Pagination *struct {
		PageNumber int32 `json:"pageNumber"`
		ShowNumber int32 `json:"showNumber"`
	} `json:"pagination"`
}

func (x *ListActiveConferencesReq) NormalizePage() (pageNumber, showNumber int32) {
	pageNumber, showNumber = x.PageNumber, x.ShowNumber
	if x.Pagination != nil {
		pageNumber = x.Pagination.PageNumber
		showNumber = x.Pagination.ShowNumber
	}
	if pageNumber < 1 {
		pageNumber = 1
	}
	if showNumber < 1 {
		showNumber = 50
	}
	if showNumber > 100 {
		showNumber = 100
	}
	return pageNumber, showNumber
}

type ActiveConferenceItem struct {
	RoomID           string `json:"roomID"`
	Title            string `json:"title,omitempty"`
	CallType         string `json:"callType"`
	CreatorUserID    string `json:"creatorUserID"`
	AdminUserID      string `json:"adminUserID"`
	ParticipantCount int    `json:"participantCount"`
	CreateTime       int64  `json:"createTime"`
	Locked           bool   `json:"locked"`
	HasPassword      bool   `json:"hasPassword"`
}

type ListActiveConferencesResp struct {
	Total       int64                   `json:"total"`
	Conferences []*ActiveConferenceItem `json:"conferences"`
}

// ─── Admin conference APIs ───────────────────────────────────────────────────

type AdminListActiveConferencesReq struct {
	PageNumber int32 `json:"pageNumber"`
	ShowNumber int32 `json:"showNumber"`
	Pagination *struct {
		PageNumber int32 `json:"pageNumber"`
		ShowNumber int32 `json:"showNumber"`
	} `json:"pagination"`
}

func (x *AdminListActiveConferencesReq) NormalizePage() (pageNumber, showNumber int32) {
	pageNumber, showNumber = x.PageNumber, x.ShowNumber
	if x.Pagination != nil {
		pageNumber = x.Pagination.PageNumber
		showNumber = x.Pagination.ShowNumber
	}
	if pageNumber < 1 {
		pageNumber = 1
	}
	if showNumber < 1 {
		showNumber = 20
	}
	return pageNumber, showNumber
}

type AdminActiveConferenceItem struct {
	RoomID               string `json:"roomID"`
	Title                string `json:"title,omitempty"`
	AdminUserID          string `json:"adminUserID"`
	CreatorUserID        string `json:"creatorUserID"`
	CallType             string `json:"callType"`
	GroupID              string `json:"groupID,omitempty"`
	CreateTime           int64  `json:"createTime"`
	RealParticipantCount int    `json:"realParticipantCount"`
	DisplayBoost         int    `json:"displayBoost"`
	ParticipantCount     int    `json:"participantCount"`
	Locked               bool   `json:"locked"`
	Rev                  int64  `json:"rev"`
}

type AdminListActiveConferencesResp struct {
	Total       int64                        `json:"total"`
	Conferences []*AdminActiveConferenceItem `json:"conferences"`
}

type AdminAdjustConferenceDisplayCountReq struct {
	RoomID   string `json:"roomID"`
	AddCount int    `json:"addCount"`
}

func (x *AdminAdjustConferenceDisplayCountReq) Check() error {
	if x.AddCount == 0 {
		return errs.ErrArgs.WrapMsg("addCount must be non-zero")
	}
	return CheckConferenceRoomID(x.RoomID)
}

type AdminAdjustConferenceDisplayCountResp struct {
	RoomID               string `json:"roomID"`
	RealParticipantCount int    `json:"realParticipantCount"`
	DisplayBoost         int    `json:"displayBoost"`
	ParticipantCount     int    `json:"participantCount"`
	Rev                  int64  `json:"rev"`
	NotifiedCount        int    `json:"notifiedCount"`
	NotifyFailed         int    `json:"notifyFailed"`
}

type AdminForceEndConferenceReq struct {
	RoomID string `json:"roomID"`
}

func (x *AdminForceEndConferenceReq) Check() error {
	return CheckConferenceRoomID(x.RoomID)
}

type AdminForceEndConferenceResp struct {
	RoomID string `json:"roomID"`
}
