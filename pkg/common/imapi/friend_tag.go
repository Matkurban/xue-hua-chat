package imapi

import "time"

type FriendTag struct {
	TagID       string    `json:"tagID"`
	OwnerUserID string    `json:"ownerUserID"`
	Name        string    `json:"name"`
	CreateTime  time.Time `json:"createTime"`
	UpdateTime  time.Time `json:"updateTime"`
}

type FriendTagMember struct {
	TagID        string    `json:"tagID"`
	OwnerUserID  string    `json:"ownerUserID"`
	FriendUserID string    `json:"friendUserID"`
	CreateTime   time.Time `json:"createTime"`
}

type CreateFriendTagReq struct {
	OwnerUserID string `json:"ownerUserID"`
	Name        string `json:"name"`
}

type CreateFriendTagResp struct {
	Tag *FriendTag `json:"tag"`
}

type RenameFriendTagReq struct {
	OwnerUserID string `json:"ownerUserID"`
	TagID       string `json:"tagID"`
	Name        string `json:"name"`
}

type DeleteFriendTagReq struct {
	OwnerUserID string `json:"ownerUserID"`
	TagID       string `json:"tagID"`
}

type AddFriendTagMembersReq struct {
	OwnerUserID   string   `json:"ownerUserID"`
	TagID         string   `json:"tagID"`
	FriendUserIDs []string `json:"friendUserIDs"`
}

type RemoveFriendTagMembersReq struct {
	OwnerUserID   string   `json:"ownerUserID"`
	TagID         string   `json:"tagID"`
	FriendUserIDs []string `json:"friendUserIDs"`
}

type ListFriendTagsReq struct {
	OwnerUserID string `json:"ownerUserID"`
	PageReq
}

type ListFriendTagsResp struct {
	Total int64        `json:"total"`
	Tags  []*FriendTag `json:"tags"`
}

type ListFriendTagMembersReq struct {
	OwnerUserID string `json:"ownerUserID"`
	TagID       string `json:"tagID"`
	PageReq
}

type ListFriendTagMembersResp struct {
	Total   int64              `json:"total"`
	Members []*FriendTagMember `json:"members"`
}
