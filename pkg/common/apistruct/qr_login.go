package apistruct

type QrLoginCreateReq struct {
	Platform int32  `json:"platform" binding:"required"`
	DeviceID string `json:"deviceID"`
}

type QrLoginCreateResp struct {
	SessionID string `json:"sessionID"`
	ExpireAt  int64  `json:"expireAt"`
}

type QrLoginStatusResp struct {
	Status    string `json:"status"`
	UserID    string `json:"userID,omitempty"`
	ImToken   string `json:"imToken,omitempty"`
	ChatToken string `json:"chatToken,omitempty"`
	Platform  int32  `json:"platform,omitempty"`
	ClientIP  string `json:"clientIP,omitempty"`
}

type QrLoginSessionReq struct {
	SessionID string `json:"sessionID" binding:"required"`
}

type QrLoginConfirmReq struct {
	SessionID string `json:"sessionID" binding:"required"`
}

type QrLoginConfirmResp struct {
	Platform int32  `json:"platform"`
	ClientIP string `json:"clientIP"`
	DeviceID string `json:"deviceID"`
}
