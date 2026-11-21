package chat

import (
	"context"
	"time"

	"chat/pkg/common/db/pagination"
)

// 红包类型
const (
	RedPacketTypeSingle = 0 // 单人（专属）红包
	RedPacketTypeLuck   = 1 // 拼手气红包

)

// 红包状态
const (
	RedPacketStatusActive    = 0 // 可领取
	RedPacketStatusExhausted = 1 // 已领完
	RedPacketStatusExpired   = 2 // 已过期（积分已退回）
)

// RedPacket 红包主表
type RedPacket struct {
	ExpireAt      time.Time `bson:"expire_at"` // 过期时间（24h后）
	CreateTime    time.Time `bson:"create_time"`
	UpdateTime    time.Time `bson:"update_time"`
	PacketID      string    `bson:"packet_id"`      // 红包唯一ID
	SenderID      string    `bson:"sender_id"`      // 发送者userID
	ConvID        string    `bson:"conv_id"`        // 所属会话ID
	Greeting      string    `bson:"greeting"`       // 祝福语
	TargetUserID  string    `bson:"target_user_id"` // 单人红包目标userID
	TotalAmount   float64   `bson:"total_amount"`   // 总积分
	GrabbedAmount float64   `bson:"grabbed_amount"` // 已被领取积分
	PacketType    int32     `bson:"packet_type"`    // 0=单人 1=拼手气 2=均等
	TotalCount    int32     `bson:"total_count"`    // 总份数
	GrabbedCount  int32     `bson:"grabbed_count"`  // 已被领取份数
	Status        int32     `bson:"status"`         // 0=可领取 1=已领完 2=已过期
}

func (RedPacket) TableName() string {
	return "red_packets"
}

// RedPacketGrab 领取记录表
type RedPacketGrab struct {
	GrabID     string    `bson:"grab_id"`
	PacketID   string    `bson:"packet_id"`
	GrabberID  string    `bson:"grabber_id"` // 领取者userID
	Amount     float64   `bson:"amount"`     // 领取金额
	CreateTime time.Time `bson:"create_time"`
}

func (RedPacketGrab) TableName() string {
	return "red_packet_grabs"
}

type RedPacketInterface interface {
	Create(ctx context.Context, packet *RedPacket) error
	TakeByPacketID(ctx context.Context, packetID string) (*RedPacket, error)
	UpdateStatus(ctx context.Context, packetID string, status int32) error
	CompareAndUpdateStatus(ctx context.Context, packetID string, fromStatus, toStatus int32) (bool, error)
	IncrGrabbed(ctx context.Context, packetID string, amount float64) error
	FindExpiredActive(ctx context.Context, before time.Time) ([]*RedPacket, error)
	SearchBySender(ctx context.Context, senderID string, pagination pagination.Pagination) (int64, []*RedPacket, error)
	SearchAll(ctx context.Context, keyword string, pagination pagination.Pagination) (int64, []*RedPacket, error)
}

type RedPacketGrabInterface interface {
	Create(ctx context.Context, grab *RedPacketGrab) error
	FindByPacketID(ctx context.Context, packetID string) ([]*RedPacketGrab, error)
	TakeByGrabber(ctx context.Context, packetID string, grabberID string) (*RedPacketGrab, error)
	SearchAll(ctx context.Context, keyword string, pagination pagination.Pagination) (int64, []*RedPacketGrab, error)
}
