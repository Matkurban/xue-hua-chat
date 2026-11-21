package database

import (
	"context"
	"time"

	"chat/pkg/common/db/mongoutil"
	"chat/pkg/common/db/pagination"
	"chat/pkg/common/db/tx"

	chatmodel "chat/pkg/common/db/model/chat"
	chatdb "chat/pkg/common/db/table/chat"
)

// RedPacketDatabaseInterface 红包+积分数据库操作接口
type RedPacketDatabaseInterface interface {
	// 红包主体
	CreateRedPacket(ctx context.Context, packet *chatdb.RedPacket) error
	GetRedPacket(ctx context.Context, packetID string) (*chatdb.RedPacket, error)
	MarkRedPacketExhausted(ctx context.Context, packetID string) error
	MarkRedPacketExpired(ctx context.Context, packetID string) error
	IncrRedPacketGrabbed(ctx context.Context, packetID string, amount float64) error
	FindExpiredRedPackets(ctx context.Context, before time.Time) ([]*chatdb.RedPacket, error)
	SearchRedPackets(ctx context.Context, keyword string, pagination pagination.Pagination) (int64, []*chatdb.RedPacket, error)

	// 领取记录
	CreateGrabRecord(ctx context.Context, grab *chatdb.RedPacketGrab) error
	GetGrabRecord(ctx context.Context, packetID string, grabberID string) (*chatdb.RedPacketGrab, error)
	ListGrabsByPacket(ctx context.Context, packetID string) ([]*chatdb.RedPacketGrab, error)
	SearchGrabs(ctx context.Context, keyword string, pagination pagination.Pagination) (int64, []*chatdb.RedPacketGrab, error)

	// 积分
	GetOrCreatePoints(ctx context.Context, userID string) (*chatdb.UserPoints, error)
	GetPointsBalance(ctx context.Context, userID string) (float64, error)
	// GrabRedPacketTx 事务：扣减发包人/增加领包人积分，同时写入流水记录
	GrabRedPacketTx(ctx context.Context, packet *chatdb.RedPacket, grab *chatdb.RedPacketGrab, senderTx *chatdb.PointsTransaction, grabberTx *chatdb.PointsTransaction) error
	// SendRedPacketTx 事务：扣减发包人积分 + 写流水 + 写红包记录
	SendRedPacketTx(ctx context.Context, packet *chatdb.RedPacket, senderTx *chatdb.PointsTransaction) error
	// RefundRedPacketTx 事务：退还积分给发送人 + 写流水 + 标记过期
	RefundRedPacketTx(ctx context.Context, packetID string, senderID string, refundAmount float64, refundTx *chatdb.PointsTransaction) error
	// AdminAdjustPoints 管理员调整积分 + 写流水
	AdminAdjustPoints(ctx context.Context, userID string, amount float64, adminTx *chatdb.PointsTransaction) error

	// 积分流水查询
	GetUserPointsTransactions(ctx context.Context, userID string, pagination pagination.Pagination) (int64, []*chatdb.PointsTransaction, error)
	SearchAllTransactions(ctx context.Context, userID string, txType int32, pagination pagination.Pagination) (int64, []*chatdb.PointsTransaction, error)
	SearchUserPoints(ctx context.Context, keyword string, pagination pagination.Pagination) (int64, []*chatdb.UserPoints, error)
	// SearchUserTransactionsFiltered 用户端带筛选查询流水
	SearchUserTransactionsFiltered(ctx context.Context, userID string, txType int32, startTime time.Time, endTime time.Time, keyword string, pagination pagination.Pagination) (int64, []*chatdb.PointsTransaction, error)
}

func NewRedPacketDatabase(cli *mongoutil.Client) (RedPacketDatabaseInterface, error) {
	redPacket, err := chatmodel.NewRedPacket(cli.GetDB())
	if err != nil {
		return nil, err
	}
	redPacketGrab, err := chatmodel.NewRedPacketGrab(cli.GetDB())
	if err != nil {
		return nil, err
	}
	userPoints, err := chatmodel.NewUserPoints(cli.GetDB())
	if err != nil {
		return nil, err
	}
	pointsTx, err := chatmodel.NewPointsTransaction(cli.GetDB())
	if err != nil {
		return nil, err
	}
	return &RedPacketDatabase{
		tx:            cli.GetTx(),
		redPacket:     redPacket,
		redPacketGrab: redPacketGrab,
		userPoints:    userPoints,
		pointsTx:      pointsTx,
	}, nil
}

type RedPacketDatabase struct {
	tx            tx.Tx
	redPacket     chatdb.RedPacketInterface
	redPacketGrab chatdb.RedPacketGrabInterface
	userPoints    chatdb.UserPointsInterface
	pointsTx      chatdb.PointsTransactionInterface
}

func (o *RedPacketDatabase) CreateRedPacket(ctx context.Context, packet *chatdb.RedPacket) error {
	return o.redPacket.Create(ctx, packet)
}

func (o *RedPacketDatabase) GetRedPacket(ctx context.Context, packetID string) (*chatdb.RedPacket, error) {
	return o.redPacket.TakeByPacketID(ctx, packetID)
}

func (o *RedPacketDatabase) MarkRedPacketExhausted(ctx context.Context, packetID string) error {
	return o.redPacket.UpdateStatus(ctx, packetID, chatdb.RedPacketStatusExhausted)
}

func (o *RedPacketDatabase) MarkRedPacketExpired(ctx context.Context, packetID string) error {
	return o.redPacket.UpdateStatus(ctx, packetID, chatdb.RedPacketStatusExpired)
}

func (o *RedPacketDatabase) IncrRedPacketGrabbed(ctx context.Context, packetID string, amount float64) error {
	return o.redPacket.IncrGrabbed(ctx, packetID, amount)
}

func (o *RedPacketDatabase) FindExpiredRedPackets(ctx context.Context, before time.Time) ([]*chatdb.RedPacket, error) {
	return o.redPacket.FindExpiredActive(ctx, before)
}

func (o *RedPacketDatabase) SearchRedPackets(ctx context.Context, keyword string, p pagination.Pagination) (int64, []*chatdb.RedPacket, error) {
	return o.redPacket.SearchAll(ctx, keyword, p)
}

func (o *RedPacketDatabase) CreateGrabRecord(ctx context.Context, grab *chatdb.RedPacketGrab) error {
	return o.redPacketGrab.Create(ctx, grab)
}

func (o *RedPacketDatabase) GetGrabRecord(ctx context.Context, packetID string, grabberID string) (*chatdb.RedPacketGrab, error) {
	return o.redPacketGrab.TakeByGrabber(ctx, packetID, grabberID)
}

func (o *RedPacketDatabase) ListGrabsByPacket(ctx context.Context, packetID string) ([]*chatdb.RedPacketGrab, error) {
	return o.redPacketGrab.FindByPacketID(ctx, packetID)
}

func (o *RedPacketDatabase) SearchGrabs(ctx context.Context, keyword string, p pagination.Pagination) (int64, []*chatdb.RedPacketGrab, error) {
	return o.redPacketGrab.SearchAll(ctx, keyword, p)
}

func (o *RedPacketDatabase) GetOrCreatePoints(ctx context.Context, userID string) (*chatdb.UserPoints, error) {
	return o.userPoints.GetOrCreate(ctx, userID)
}

func (o *RedPacketDatabase) GetPointsBalance(ctx context.Context, userID string) (float64, error) {
	return o.userPoints.GetBalance(ctx, userID)
}

// SendRedPacketTx 发红包事务：扣积分 + 写流水 + 创建红包
func (o *RedPacketDatabase) SendRedPacketTx(ctx context.Context, packet *chatdb.RedPacket, senderTx *chatdb.PointsTransaction) error {
	return o.tx.Transaction(ctx, func(ctx context.Context) error {
		if err := o.userPoints.IncrBalance(ctx, packet.SenderID, -packet.TotalAmount, true); err != nil {
			return err
		}
		if err := o.pointsTx.Create(ctx, senderTx); err != nil {
			return err
		}
		return o.redPacket.Create(ctx, packet)
	})
}

// GrabRedPacketTx 抢红包事务：增加领取人积分 + 写双方流水 + 更新红包已领金额
func (o *RedPacketDatabase) GrabRedPacketTx(ctx context.Context, packet *chatdb.RedPacket, grab *chatdb.RedPacketGrab, senderTx *chatdb.PointsTransaction, grabberTx *chatdb.PointsTransaction) error {
	return o.tx.Transaction(ctx, func(ctx context.Context) error {
		if err := o.redPacketGrab.Create(ctx, grab); err != nil {
			return err
		}
		if err := o.redPacket.IncrGrabbed(ctx, packet.PacketID, grab.Amount); err != nil {
			return err
		}
		if err := o.userPoints.IncrBalance(ctx, grab.GrabberID, grab.Amount, false); err != nil {
			return err
		}
		if err := o.pointsTx.Create(ctx, grabberTx); err != nil {
			return err
		}
		return nil
	})
}

// RefundRedPacketTx 退款事务：先原子将活跃红包置为已过期，再退积分给发包人 + 写流水，防止多实例并发重复退款
func (o *RedPacketDatabase) RefundRedPacketTx(ctx context.Context, packetID string, senderID string, refundAmount float64, refundTx *chatdb.PointsTransaction) error {
	return o.tx.Transaction(ctx, func(ctx context.Context) error {
		updated, err := o.redPacket.CompareAndUpdateStatus(ctx, packetID, chatdb.RedPacketStatusActive, chatdb.RedPacketStatusExpired)
		if err != nil {
			return err
		}
		if !updated {
			return nil
		}
		if err := o.userPoints.IncrBalance(ctx, senderID, refundAmount, false); err != nil {
			return err
		}
		return o.pointsTx.Create(ctx, refundTx)
	})
}

// AdminAdjustPoints 管理员调整积分 + 写流水
func (o *RedPacketDatabase) AdminAdjustPoints(ctx context.Context, userID string, amount float64, adminTx *chatdb.PointsTransaction) error {
	return o.tx.Transaction(ctx, func(ctx context.Context) error {
		if err := o.userPoints.AdminAdjust(ctx, userID, amount); err != nil {
			return err
		}
		return o.pointsTx.Create(ctx, adminTx)
	})
}

func (o *RedPacketDatabase) GetUserPointsTransactions(ctx context.Context, userID string, p pagination.Pagination) (int64, []*chatdb.PointsTransaction, error) {
	return o.pointsTx.FindByUser(ctx, userID, p)
}

func (o *RedPacketDatabase) SearchAllTransactions(ctx context.Context, userID string, txType int32, p pagination.Pagination) (int64, []*chatdb.PointsTransaction, error) {
	return o.pointsTx.SearchFiltered(ctx, userID, txType, p)
}

func (o *RedPacketDatabase) SearchUserPoints(ctx context.Context, keyword string, p pagination.Pagination) (int64, []*chatdb.UserPoints, error) {
	return o.userPoints.SearchAll(ctx, keyword, p)
}

func (o *RedPacketDatabase) SearchUserTransactionsFiltered(ctx context.Context, userID string, txType int32, startTime time.Time, endTime time.Time, keyword string, p pagination.Pagination) (int64, []*chatdb.PointsTransaction, error) {
	return o.pointsTx.SearchUserFiltered(ctx, userID, txType, startTime, endTime, keyword, p)
}
