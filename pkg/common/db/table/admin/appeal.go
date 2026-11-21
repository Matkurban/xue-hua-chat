package admin

import (
	"context"
	"time"

	"chat/pkg/common/db/pagination"
)

// Appeal 用户封禁申诉
type Appeal struct {
	AppealID    string    `bson:"appeal_id"`
	Account     string    `bson:"account"` // 申诉人账号 (userID/手机/邮箱)
	UserID      string    `bson:"user_id,omitempty"`
	Reason      string    `bson:"reason"` // mistaken / unfair / other
	Description string    `bson:"description"`
	Evidence    []string  `bson:"evidence"`
	Contact     string    `bson:"contact,omitempty"`
	Status      string    `bson:"status"` // pending / reviewing / approved / rejected
	HandlerID   string    `bson:"handler_id,omitempty"`
	HandleNote  string    `bson:"handle_note,omitempty"`
	ClientIP    string    `bson:"client_ip,omitempty"`
	CreateTime  time.Time `bson:"create_time"`
	HandleTime  time.Time `bson:"handle_time,omitempty"`
}

func (Appeal) TableName() string {
	return "appeals"
}

type AppealInterface interface {
	Create(ctx context.Context, ms []*Appeal) error
	Take(ctx context.Context, appealID string) (*Appeal, error)
	Update(ctx context.Context, appealID string, update map[string]any) error
	AddEvidence(ctx context.Context, appealID string, url string) error
	Search(ctx context.Context, status string, keyword string, pagination pagination.Pagination) (int64, []*Appeal, error)
	CountRecent(ctx context.Context, account string, ip string, since time.Time) (int64, error)
}
