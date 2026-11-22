package imapi

import "time"

type PageReq struct {
	PageNumber int32 `json:"pageNumber"`
	ShowNumber int32 `json:"showNumber"`
}

type MomentMediaItem struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	CoverURL string `json:"coverURL"`
	Duration int64  `json:"duration"`
	Extra    string `json:"extra"`
}

// MomentUserInfo contains basic user info for display in moments
type MomentUserInfo struct {
	UserID   string `json:"userID"`
	Nickname string `json:"nickname"`
	FaceURL  string `json:"faceURL"`
}

type Moment struct {
	CreateTime      time.Time         `json:"createTime"`
	UpdateTime      time.Time         `json:"updateTime"`
	MomentID        string            `json:"momentID"`
	UserID          string            `json:"userID"`
	Content         string            `json:"content"`
	Extra           string            `json:"extra"`
	Media           []MomentMediaItem `json:"media"`
	VisibleGroupIDs []string          `json:"visibleGroupIDs"`
	LikeCount       int64             `json:"likeCount"`
	CommentCount    int64             `json:"commentCount"`
	VisibleType     int32             `json:"visibleType"`
	Status          int32             `json:"status"`
	TimelineSeq     int64             `json:"timelineSeq"`
	UpdateSeq       int64             `json:"updateSeq"`
}

type MomentComment struct {
	CommentID     string    `json:"commentID"`
	MomentID      string    `json:"momentID"`
	UserID        string    `json:"userID"`
	ReplyToUserID string    `json:"replyToUserID"`
	Content       string    `json:"content"`
	Status        int32     `json:"status"`
	CreateTime    time.Time `json:"createTime"`
	UpdateTime    time.Time `json:"updateTime"`
}

type MomentLike struct {
	MomentID   string    `json:"momentID"`
	UserID     string    `json:"userID"`
	CreateTime time.Time `json:"createTime"`
}

// MomentCommentWithUser contains comment with user info
type MomentCommentWithUser struct {
	CommentID     string          `json:"commentID"`
	MomentID      string          `json:"momentID"`
	UserID        string          `json:"userID"`
	ReplyToUserID string          `json:"replyToUserID"`
	Content       string          `json:"content"`
	Status        int32           `json:"status"`
	CreateTime    time.Time       `json:"createTime"`
	UpdateTime    time.Time       `json:"updateTime"`
	UserInfo      *MomentUserInfo `json:"userInfo"`
	ReplyToUser   *MomentUserInfo `json:"replyToUser,omitempty"`
}

// MomentLikeWithUser contains like with user info
type MomentLikeWithUser struct {
	MomentID   string          `json:"momentID"`
	UserID     string          `json:"userID"`
	CreateTime time.Time       `json:"createTime"`
	UserInfo   *MomentUserInfo `json:"userInfo"`
}

// MomentWithInteractions contains moment with likes and comments
type MomentWithInteractions struct {
	*Moment
	UserInfo *MomentUserInfo          `json:"userInfo,omitempty"`
	Likes    []*MomentLikeWithUser    `json:"likes"`
	Comments []*MomentCommentWithUser `json:"comments"`
}

type CreateMomentReq struct {
	UserID          string            `json:"userID"`
	Content         string            `json:"content"`
	Media           []MomentMediaItem `json:"media"`
	VisibleType     int32             `json:"visibleType"`
	VisibleGroupIDs []string          `json:"visibleGroupIDs"`
	Extra           string            `json:"extra"`
}

type CreateMomentResp struct {
	Moment         *MomentWithInteractions `json:"moment"`
	SyncSeq        int64                   `json:"syncSeq,omitempty"`
	VisibleUserIDs []string                `json:"visibleUserIDs,omitempty"`
}

type MomentMutationResp struct {
	SyncSeq        int64               `json:"syncSeq,omitempty"`
	VisibleUserIDs []string            `json:"visibleUserIDs,omitempty"`
	Like           *MomentLikeWithUser `json:"like,omitempty"`
}

type DeleteMomentReq struct {
	UserID   string `json:"userID"`
	MomentID string `json:"momentID"`
}

type ListMomentsReq struct {
	UserID      string `json:"userID"`
	OwnerUserID string `json:"ownerUserID"`
	PageReq
}

type ListMomentsResp struct {
	Total   int64                     `json:"total"`
	Moments []*MomentWithInteractions `json:"moments"`
}

type ListTimelineReq struct {
	UserID      string `json:"userID"`
	OwnerUserID string `json:"ownerUserID"`
	BeforeSeq   int64  `json:"beforeSeq"`
	Limit       int64  `json:"limit"`
}

type ListTimelineResp struct {
	Moments       []*MomentWithInteractions `json:"moments"`
	NextBeforeSeq int64                     `json:"nextBeforeSeq"`
	HasMore       bool                      `json:"hasMore"`
	LatestSyncSeq int64                     `json:"latestSyncSeq"`
}

type SyncMomentsReq struct {
	UserID      string `json:"userID"`
	LastSyncSeq int64  `json:"lastSyncSeq"`
	Limit       int64  `json:"limit"`
}

type SyncMomentsResp struct {
	LatestSyncSeq    int64                     `json:"latestSyncSeq"`
	NeedFullRefresh  bool                      `json:"needFullRefresh"`
	HasMore          bool                      `json:"hasMore"`
	Upserts          []*MomentWithInteractions `json:"upserts"`
	DeletedMomentIDs []string                  `json:"deletedMomentIDs"`
}

type CommentMomentReq struct {
	UserID        string `json:"userID"`
	MomentID      string `json:"momentID"`
	ReplyToUserID string `json:"replyToUserID"`
	Content       string `json:"content"`
	OwnerUserID   string `json:"ownerUserID"`
}

type CommentMomentResp struct {
	Comment        *MomentCommentWithUser `json:"comment"`
	SyncSeq        int64                  `json:"syncSeq,omitempty"`
	VisibleUserIDs []string               `json:"visibleUserIDs,omitempty"`
}

type DeleteCommentReq struct {
	UserID    string `json:"userID"`
	CommentID string `json:"commentID"`
}

type DeleteCommentResp struct {
	MomentID       string   `json:"momentID,omitempty"`
	SyncSeq        int64    `json:"syncSeq,omitempty"`
	VisibleUserIDs []string `json:"visibleUserIDs,omitempty"`
}

type LikeMomentReq struct {
	UserID      string `json:"userID"`
	MomentID    string `json:"momentID"`
	OwnerUserID string `json:"ownerUserID"`
}

type UnlikeMomentReq struct {
	UserID      string `json:"userID"`
	MomentID    string `json:"momentID"`
	OwnerUserID string `json:"ownerUserID"`
}

// ---- Admin moment management (xue-hua-server /moment/admin/*) ----

// AdminMoment is a moment with its author info.
type AdminMoment struct {
	*Moment
	UserInfo *MomentUserInfo `json:"userInfo,omitempty"`
}

type AdminListMomentsReq struct {
	UserID    string `json:"userID"`
	Keyword   string `json:"keyword"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
	Status    *int32 `json:"status"`
	PageReq
}

type AdminListMomentsResp struct {
	Total   int64          `json:"total"`
	Moments []*AdminMoment `json:"moments"`
}

type AdminMomentIDReq struct {
	MomentID string `json:"momentID"`
}

type AdminMomentDetailResp struct {
	Moment *AdminMoment `json:"moment"`
}

type AdminMomentIDsReq struct {
	MomentIDs []string `json:"momentIDs"`
}

type AdminDeleteMomentsResp struct {
	DeletedCount int64 `json:"deletedCount"`
}

type AdminListCommentsReq struct {
	MomentID string `json:"momentID"`
	PageReq
}

type AdminListCommentsResp struct {
	Total    int64                    `json:"total"`
	Comments []*MomentCommentWithUser `json:"comments"`
}

type AdminDeleteCommentReq struct {
	CommentID string `json:"commentID"`
}

type AdminListLikesReq struct {
	MomentID string `json:"momentID"`
	PageReq
}

type AdminListLikesResp struct {
	Total int64                 `json:"total"`
	Likes []*MomentLikeWithUser `json:"likes"`
}

type AdminSetStatusResp struct {
	Affected int64 `json:"affected"`
}

type AdminMomentStatsReq struct{}

type AdminMomentStatsResp struct {
	TotalMoments  int64 `json:"totalMoments"`
	TodayMoments  int64 `json:"todayMoments"`
	BannedMoments int64 `json:"bannedMoments"`
	TotalComments int64 `json:"totalComments"`
	TotalLikes    int64 `json:"totalLikes"`
}
