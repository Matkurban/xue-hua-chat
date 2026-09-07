package admin

import (
	"context"
	"time"

	"gitee.com/matkurban/openimsdk-tools/db/pagination"
)

const (
	SystemNotificationStatusSent     = "sent"
	SystemNotificationStatusRevoking = "revoking"
	SystemNotificationStatusRevoked  = "revoked"
	SystemNotificationStatusPartial  = "partial"

	SystemNotificationTargetSent    = "sent"
	SystemNotificationTargetRevoked = "revoked"
	SystemNotificationTargetFailed  = "failed"
)

// SystemNotification is one admin OA broadcast (not per-recipient).
type SystemNotification struct {
	CreateTime       time.Time `bson:"create_time" json:"-"`
	RevokeTime       time.Time `bson:"revoke_time,omitempty" json:"-"`
	NotificationID   string    `bson:"notification_id" json:"notificationID"`
	SendID           string    `bson:"send_id" json:"sendID"`
	SenderNickname   string    `bson:"sender_nickname" json:"senderNickname"`
	Title            string    `bson:"title" json:"title"`
	Text             string    `bson:"text" json:"text"`
	ImageURL         string    `bson:"image_url" json:"imageUrl"`
	ExternalURL      string    `bson:"external_url" json:"externalUrl"`
	Status           string    `bson:"status" json:"status"`
	OperatorAdminID  string    `bson:"operator_admin_id" json:"operatorAdminID"`
	RevokeBy         string    `bson:"revoke_by,omitempty" json:"revokeBy,omitempty"`
	NotificationType int32     `bson:"notification_type" json:"notificationType"`
	SuccessCount     int32     `bson:"success_count" json:"successCount"`
	FailedCount      int32     `bson:"failed_count" json:"failedCount"`
	RevokeSuccess    int32     `bson:"revoke_success" json:"revokeSuccess"`
	RevokeFailed     int32     `bson:"revoke_failed" json:"revokeFailed"`
	IsSendAll        bool      `bson:"is_send_all" json:"isSendAll"`
}

func (SystemNotification) TableName() string {
	return "system_notifications"
}

type SystemNotificationInterface interface {
	Create(ctx context.Context, n *SystemNotification) error
	Take(ctx context.Context, notificationID string) (*SystemNotification, error)
	Update(ctx context.Context, notificationID string, update map[string]any) error
	Search(ctx context.Context, pagination pagination.Pagination) (int64, []*SystemNotification, error)
}

// SystemNotificationTarget is one recipient of a broadcast, used to revoke later.
type SystemNotificationTarget struct {
	NotificationID string `bson:"notification_id"`
	RecvID         string `bson:"recv_id"`
	ClientMsgID    string `bson:"client_msg_id"`
	ServerMsgID    string `bson:"server_msg_id"`
	SendTime       int64  `bson:"send_time"`
	Status         string `bson:"status"`
}

func (SystemNotificationTarget) TableName() string {
	return "system_notification_targets"
}

type SystemNotificationTargetInterface interface {
	CreateMany(ctx context.Context, items []*SystemNotificationTarget) error
	ListByNotification(ctx context.Context, notificationID string, skip, limit int64) ([]*SystemNotificationTarget, error)
	CountByNotification(ctx context.Context, notificationID string, status string) (int64, error)
	MarkStatus(ctx context.Context, notificationID string, recvIDs []string, status string) error
}
