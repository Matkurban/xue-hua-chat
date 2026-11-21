package chat

import (
	"context"
	"math"
	"time"

	rpCache "chat/pkg/common/db/cache"
	"chat/pkg/common/db/database"
	chatdb "chat/pkg/common/db/table/chat"
	"chat/pkg/common/imapi"
	"chat/pkg/common/log"
	"github.com/google/uuid"
)

// startExpireRefundLoop 每5分钟扫描过期红包，退还未领完积分给发包人
func startExpireRefundLoop(ctx context.Context, db database.RedPacketDatabaseInterface, cache *rpCache.RedPacketCache, im imapi.CallerInterface, imUserID string) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processExpiredRedPackets(ctx, db, cache, im, imUserID)
		}
	}
}

func processExpiredRedPackets(ctx context.Context, db database.RedPacketDatabaseInterface, cache *rpCache.RedPacketCache, im imapi.CallerInterface, imUserID string) {
	expired, err := db.FindExpiredRedPackets(ctx, time.Now())
	if err != nil {
		log.ZError(ctx, "FindExpiredRedPackets error", err)
		return
	}
	for _, packet := range expired {
		refund := math.Round((packet.TotalAmount-packet.GrabbedAmount)*100) / 100
		if refund <= 0 {
			// 无需退款，直接标记过期
			_ = db.MarkRedPacketExpired(ctx, packet.PacketID)
			continue
		}
		refundTx := &chatdb.PointsTransaction{
			TxID:       uuid.New().String(),
			UserID:     packet.SenderID,
			Amount:     refund,
			TxType:     chatdb.PointsTxTypeRefund,
			RelatedID:  packet.PacketID,
			Remark:     "红包过期退回",
			CreateTime: time.Now(),
		}
		if err = db.RefundRedPacketTx(ctx, packet.PacketID, packet.SenderID, refund, refundTx); err != nil {
			log.ZError(ctx, "RefundRedPacketTx failed", err, "packetID", packet.PacketID)
			continue
		}
		// 清理 Redis 缓存
		if cache != nil {
			cache.DeletePacket(ctx, packet.PacketID)
		}
		// 发送退款通知给发包人
		if im != nil {
			_ = im.SendBusinessNotification(ctx, "red_packet_expired", packet.PacketID,
				packet.SenderID, packet.SenderID)
		}
		log.ZInfo(ctx, "red packet expired and refunded",
			"packetID", packet.PacketID,
			"senderID", packet.SenderID,
			"refundAmount", refund)
	}
}
