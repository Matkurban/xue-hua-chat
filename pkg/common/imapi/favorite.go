package imapi

import "time"

type Favorite struct {
	FavoriteID string    `json:"favoriteID"`
	UserID     string    `json:"userID"`
	TargetType string    `json:"targetType"`
	TargetID   string    `json:"targetID"`
	Data       string    `json:"data"`
	CreateTime time.Time `json:"createTime"`
}

type AddFavoriteReq struct {
	UserID     string `json:"userID"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetID"`
	Data       string `json:"data"`
}

type AddFavoriteResp struct {
	Favorite *Favorite `json:"favorite"`
}

type RemoveFavoriteReq struct {
	UserID     string `json:"userID"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetID"`
}

type ListFavoritesReq struct {
	UserID string `json:"userID"`
	PageReq
}

type ListFavoritesResp struct {
	Total     int64       `json:"total"`
	Favorites []*Favorite `json:"favorites"`
}

type UpdateFavoriteReq struct {
	UserID     string `json:"userID"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetID"`
	Data       string `json:"data"`
}

type UpdateFavoriteResp struct {
	Favorite *Favorite `json:"favorite"`
}
