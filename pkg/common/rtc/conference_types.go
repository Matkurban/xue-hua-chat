package rtc

// Conference roles.
const (
	ConferenceRoleAdmin  = "admin"
	ConferenceRoleCohost = "cohost"
	ConferenceRoleUser   = "user"
)

const (
	ConferenceMaxParticipants = 500
	DefaultAutoStageLimit     = 9
)

// ConferenceAnnounceRef tracks a chat card for status updates on end.
type ConferenceAnnounceRef struct {
	ConversationID string `json:"conversationID"`
	ClientMsgID    string `json:"clientMsgID"`
	ServerMsgID    string `json:"serverMsgID,omitempty"`
}

// ConferenceRoom is the authoritative room metadata (no full member map).
type ConferenceRoom struct {
	RoomID               string `json:"roomID"`
	AdminUserID          string `json:"adminUserID"`
	CreatorUserID        string `json:"creatorUserID"`
	CallType             string `json:"callType"` // audio | video
	Title                string `json:"title,omitempty"`
	GroupID              string `json:"groupID,omitempty"`
	SourceConversationID string `json:"sourceConversationID,omitempty"`
	PinnedUserID         string `json:"pinnedUserID,omitempty"`
	EgressID             string `json:"egressID,omitempty"`
	IngressID            string `json:"ingressID,omitempty"`
	IngressURL           string `json:"ingressURL,omitempty"`
	IngressStreamKey     string `json:"ingressStreamKey,omitempty"`
	// Schedule / security (optional; set when activated from 预约会议)
	PasswordHash         string                  `json:"passwordHash,omitempty"`
	ScheduleOccurrenceID string                  `json:"scheduleOccurrenceID,omitempty"`
	InviteeUserIDs       []string                `json:"inviteeUserIDs"`
	AnnounceRefs         []ConferenceAnnounceRef `json:"announceRefs,omitempty"`
	CameraOnUserIDs      []string                `json:"cameraOnUserIDs,omitempty"` // who currently has camera on
	CreateTime           int64                   `json:"createTime"`
	ParticipantCount     int                     `json:"participantCount"`       // display = real members + DisplayBoost
	DisplayBoost         int                     `json:"displayBoost,omitempty"` // admin virtual extras (not LiveKit seats)
	Rev                  int64                   `json:"rev"`
	AutoStageLimit       int                     `json:"autoStageLimit"` // snapshot at create; hard max on-stage
	GlobalAudioMuted     bool                    `json:"globalAudioMuted"`
	GlobalVideoOff       bool                    `json:"globalVideoOff"`
	ChatMuted            bool                    `json:"chatMuted"`
	Locked               bool                    `json:"locked"`
	LinkMicDisabled      bool                    `json:"linkMicDisabled,omitempty"` // zero = 连麦开（旧房间无需迁移）
	WaitingRoomEnabled   bool                    `json:"waitingRoomEnabled,omitempty"`
}

// ActiveConferenceSummary is an admin list-row snapshot for an ongoing conference.
type ActiveConferenceSummary struct {
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
	HasPassword          bool   `json:"hasPassword"`
	Rev                  int64  `json:"rev"`
	NotifiedCount        int    `json:"notifiedCount,omitempty"`
	NotifyFailed         int    `json:"notifyFailed,omitempty"`
}

// PublicActiveConference is the client-facing snapshot of an ongoing conference.
// It never includes password hashes, invitee lists, or admin display-boost fields.
type PublicActiveConference struct {
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

// PublicActiveConferenceFromRoom builds a client snapshot.
// participantCount is the display total (real members + DisplayBoost). The
// JSON still omits the displayBoost field itself.
func PublicActiveConferenceFromRoom(room *ConferenceRoom, participantCount int) *PublicActiveConference {
	if room == nil {
		return nil
	}
	count := participantCount
	if count <= 0 {
		count = room.ParticipantCount
	}
	return &PublicActiveConference{
		RoomID:           room.RoomID,
		Title:            room.Title,
		CallType:         room.CallType,
		CreatorUserID:    room.CreatorUserID,
		AdminUserID:      room.AdminUserID,
		ParticipantCount: count,
		CreateTime:       room.CreateTime,
		Locked:           room.Locked,
		HasPassword:      room.PasswordHash != "",
	}
}

// PublicActiveConferenceFromSummary maps an admin row to the client-facing shape.
func PublicActiveConferenceFromSummary(s *ActiveConferenceSummary) *PublicActiveConference {
	if s == nil {
		return nil
	}
	return &PublicActiveConference{
		RoomID:           s.RoomID,
		Title:            s.Title,
		CallType:         s.CallType,
		CreatorUserID:    s.CreatorUserID,
		AdminUserID:      s.AdminUserID,
		ParticipantCount: s.ParticipantCount,
		CreateTime:       s.CreateTime,
		Locked:           s.Locked,
		HasPassword:      s.HasPassword,
	}
}

// RefreshParticipantCount sets ParticipantCount to real member count plus DisplayBoost.
func (r *ConferenceRoom) RefreshParticipantCount(realMemberCount int) {
	if r == nil {
		return
	}
	if r.DisplayBoost < 0 {
		r.DisplayBoost = 0
	}
	r.ParticipantCount = realMemberCount + r.DisplayBoost
}

// ConferenceMember is per-user state in a conference.
type ConferenceMember struct {
	UserID         string `json:"userID"`
	Role           string `json:"role"`
	JoinTime       int64  `json:"joinTime"`
	InLiveKit      bool   `json:"inLiveKit"`
	AudioForcedOff bool   `json:"audioForcedOff"`
	VideoForcedOff bool   `json:"videoForcedOff"`
	ChatForcedOff  bool   `json:"chatForcedOff"` // personal text/danmaku mute
	OnStage        bool   `json:"onStage"`
	CameraOn       bool   `json:"cameraOn"`
	SpeakPending   bool   `json:"speakPending,omitempty"`
	Nickname       string `json:"nickname,omitempty"`
	FaceURL        string `json:"faceURL,omitempty"`
}

// ConferenceStageState is fan-out stage/camera snapshot (JSON in sync).
type ConferenceStageState struct {
	PendingApplyUserIDs []string `json:"pendingApplyUserIDs,omitempty"`
	CameraOnUserIDs     []string `json:"cameraOnUserIDs,omitempty"`
	CameraOnCount       int      `json:"cameraOnCount"`
	AutoStageLimit      int      `json:"autoStageLimit"` // hard max on-stage seats
	OnStageCount        int      `json:"onStageCount"`
	LinkMicEnabled      bool     `json:"linkMicEnabled"`
	TargetOnStage       *bool    `json:"targetOnStage,omitempty"`
	TargetSpeakPending  *bool    `json:"targetSpeakPending,omitempty"`
	TargetCameraOn      *bool    `json:"targetCameraOn,omitempty"`
}

func roomLinkMicEnabled(room *ConferenceRoom) bool {
	return room == nil || !room.LinkMicDisabled
}

// ConferenceParticipantPage is a paginated member list response.
type ConferenceParticipantPage struct {
	Total      int64               `json:"total"`
	PageNumber int32               `json:"pageNumber"`
	PageSize   int32               `json:"pageSize"`
	List       []*ConferenceMember `json:"list"`
}
