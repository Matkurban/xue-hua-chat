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
	Moment *MomentWithInteractions `json:"moment"`
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

type CommentMomentReq struct {
	UserID        string `json:"userID"`
	MomentID      string `json:"momentID"`
	ReplyToUserID string `json:"replyToUserID"`
	Content       string `json:"content"`
	OwnerUserID   string `json:"ownerUserID"`
}

type CommentMomentResp struct {
	Comment *MomentComment `json:"comment"`
}

type DeleteCommentReq struct {
	UserID    string `json:"userID"`
	CommentID string `json:"commentID"`
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
