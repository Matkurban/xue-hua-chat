package admin

import (
	"context"
	"time"

	"chat/pkg/common/db/pagination"
)

// Report 用户举报工单
type Report struct {
	ReportID     string    `bson:"report_id"`
	ReporterID   string    `bson:"reporter_id"`
	TargetType   string    `bson:"target_type"` // user / group / message
	TargetID     string    `bson:"target_id"`
	MessageID    string    `bson:"message_id,omitempty"`
	Category     string    `bson:"category"` // harass / scam / porn / ad / other
	Description  string    `bson:"description"`
	Evidence     []string  `bson:"evidence"`
	Status       string    `bson:"status"` // pending / processing / resolved / rejected
	HandlerID    string    `bson:"handler_id,omitempty"`
	HandleNote   string    `bson:"handle_note,omitempty"`
	HandleAction string    `bson:"handle_action,omitempty"` // none / block_user / delete_content
	CreateTime   time.Time `bson:"create_time"`
	HandleTime   time.Time `bson:"handle_time,omitempty"`
}

func (Report) TableName() string {
	return "reports"
}

type ReportInterface interface {
	Create(ctx context.Context, ms []*Report) error
	Take(ctx context.Context, reportID string) (*Report, error)
	Update(ctx context.Context, reportID string, update map[string]any) error
	Search(ctx context.Context, status string, category string, keyword string, pagination pagination.Pagination) (int64, []*Report, error)
}
