package chat

import (
	"context"
	"time"

	"chat/pkg/common/db/pagination"
)

// 积分余额表（每用户一条文档）
type UserPoints struct {
	UserID     string    `bson:"user_id"     json:"userID"`
	Balance    float64   `bson:"balance"     json:"balance"`
	CreateTime time.Time `bson:"create_time" json:"createTime"`
	UpdateTime time.Time `bson:"update_time" json:"updateTime"`
}

func (UserPoints) TableName() string {
	return "user_points"
}

// 积分交易流水类型
const (
	PointsTxTypeSend     = 1 // 发红包扣积分
	PointsTxTypeReceive  = 2 // 领红包得积分
	PointsTxTypeRefund   = 3 // 红包过期退积分
	PointsTxTypeAdminAdd = 4 // 管理员增加积分
	PointsTxTypeAdminSub = 5 // 管理员减少积分
)

// PointsTransaction 积分流水表
type PointsTransaction struct {
	TxID       string    `bson:"tx_id"       json:"txID"`
	UserID     string    `bson:"user_id"     json:"userID"`
	Amount     float64   `bson:"amount"      json:"amount"`
	TxType     int32     `bson:"tx_type"     json:"txType"`
	RelatedID  string    `bson:"related_id"  json:"relatedID"`
	Remark     string    `bson:"remark"      json:"remark"`
	CreateTime time.Time `bson:"create_time" json:"createTime"`
}

func (PointsTransaction) TableName() string {
	return "points_transactions"
}

type UserPointsInterface interface {
	Create(ctx context.Context, up *UserPoints) error
	GetOrCreate(ctx context.Context, userID string) (*UserPoints, error)
	GetBalance(ctx context.Context, userID string) (float64, error)
	// IncrBalance 原子增减，amount可负，debitCheck=true时检查余额不足
	IncrBalance(ctx context.Context, userID string, amount float64, debitCheck bool) error
	AdminAdjust(ctx context.Context, userID string, amount float64) error
	SearchAll(ctx context.Context, keyword string, pagination pagination.Pagination) (int64, []*UserPoints, error)
}

type PointsTransactionInterface interface {
	Create(ctx context.Context, tx *PointsTransaction) error
	FindByUser(ctx context.Context, userID string, pagination pagination.Pagination) (int64, []*PointsTransaction, error)
	SearchAll(ctx context.Context, keyword string, pagination pagination.Pagination) (int64, []*PointsTransaction, error)
	SearchFiltered(ctx context.Context, userID string, txType int32, pagination pagination.Pagination) (int64, []*PointsTransaction, error)
	// SearchUserFiltered 用户端带筛选查询：txType、时间范围、关键字（匹配 relatedID/remark）
	SearchUserFiltered(ctx context.Context, userID string, txType int32, startTime time.Time, endTime time.Time, keyword string, pagination pagination.Pagination) (int64, []*PointsTransaction, error)
}
