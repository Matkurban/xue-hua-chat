package chat

import (
	"context"
	"regexp"
	"time"

	"chat/pkg/common/db/mongoutil"
	"chat/pkg/common/db/pagination"
	"chat/pkg/common/errs"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"chat/pkg/common/db/table/chat"
)

func NewRedPacket(db *mongo.Database) (chat.RedPacketInterface, error) {
	coll := db.Collection("red_packets")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "packet_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "sender_id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "expire_at", Value: 1}}},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &RedPacket{coll: coll}, nil
}

type RedPacket struct {
	coll *mongo.Collection
}

func (o *RedPacket) Create(ctx context.Context, packet *chat.RedPacket) error {
	return mongoutil.InsertMany(ctx, o.coll, []*chat.RedPacket{packet})
}

func (o *RedPacket) TakeByPacketID(ctx context.Context, packetID string) (*chat.RedPacket, error) {
	return mongoutil.FindOne[*chat.RedPacket](ctx, o.coll, bson.M{"packet_id": packetID})
}

func (o *RedPacket) UpdateStatus(ctx context.Context, packetID string, status int32) error {
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"packet_id": packetID},
		bson.M{"$set": bson.M{"status": status, "update_time": time.Now()}}, false)
}

func (o *RedPacket) CompareAndUpdateStatus(ctx context.Context, packetID string, fromStatus, toStatus int32) (bool, error) {
	res, err := o.coll.UpdateOne(ctx,
		bson.M{"packet_id": packetID, "status": fromStatus},
		bson.M{"$set": bson.M{"status": toStatus, "update_time": time.Now()}},
	)
	if err != nil {
		return false, errs.WrapMsg(err, "CompareAndUpdateStatus failed")
	}
	return res.MatchedCount > 0, nil
}

func (o *RedPacket) IncrGrabbed(ctx context.Context, packetID string, amount float64) error {
	_, err := o.coll.UpdateOne(ctx,
		bson.M{"packet_id": packetID},
		bson.M{
			"$inc": bson.M{"grabbed_amount": amount, "grabbed_count": 1},
			"$set": bson.M{"update_time": time.Now()},
		},
	)
	return errs.WrapMsg(err, "IncrGrabbed failed")
}

func (o *RedPacket) FindExpiredActive(ctx context.Context, before time.Time) ([]*chat.RedPacket, error) {
	return mongoutil.Find[*chat.RedPacket](ctx, o.coll,
		bson.M{"status": chat.RedPacketStatusActive, "expire_at": bson.M{"$lte": before}})
}

func (o *RedPacket) SearchBySender(ctx context.Context, senderID string, p pagination.Pagination) (int64, []*chat.RedPacket, error) {
	return mongoutil.FindPage[*chat.RedPacket](ctx, o.coll,
		bson.M{"sender_id": senderID},
		p,
		options.Find().SetSort(bson.D{{Key: "create_time", Value: -1}}))
}

func (o *RedPacket) SearchAll(ctx context.Context, keyword string, p pagination.Pagination) (int64, []*chat.RedPacket, error) {
	filter := bson.M{}
	if keyword != "" {
		quoted := regexp.QuoteMeta(keyword)
		filter["$or"] = bson.A{
			bson.M{"packet_id": bson.M{"$regex": quoted}},
			bson.M{"sender_id": bson.M{"$regex": quoted}},
		}
	}
	return mongoutil.FindPage[*chat.RedPacket](ctx, o.coll, filter, p,
		options.Find().SetSort(bson.D{{Key: "create_time", Value: -1}}))
}

// ─── RedPacketGrab ───────────────────────────────────────────────────────────

func NewRedPacketGrab(db *mongo.Database) (chat.RedPacketGrabInterface, error) {
	coll := db.Collection("red_packet_grabs")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "grab_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "packet_id", Value: 1}}},
		// 保证同一用户对同一红包只能领一次
		{Keys: bson.D{{Key: "packet_id", Value: 1}, {Key: "grabber_id", Value: 1}},
			Options: options.Index().SetUnique(true)},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &RedPacketGrab{coll: coll}, nil
}

type RedPacketGrab struct {
	coll *mongo.Collection
}

func (o *RedPacketGrab) Create(ctx context.Context, grab *chat.RedPacketGrab) error {
	return mongoutil.InsertMany(ctx, o.coll, []*chat.RedPacketGrab{grab})
}

func (o *RedPacketGrab) FindByPacketID(ctx context.Context, packetID string) ([]*chat.RedPacketGrab, error) {
	return mongoutil.Find[*chat.RedPacketGrab](ctx, o.coll, bson.M{"packet_id": packetID})
}

func (o *RedPacketGrab) TakeByGrabber(ctx context.Context, packetID string, grabberID string) (*chat.RedPacketGrab, error) {
	return mongoutil.FindOne[*chat.RedPacketGrab](ctx, o.coll,
		bson.M{"packet_id": packetID, "grabber_id": grabberID})
}

func (o *RedPacketGrab) SearchAll(ctx context.Context, keyword string, p pagination.Pagination) (int64, []*chat.RedPacketGrab, error) {
	filter := bson.M{}
	if keyword != "" {
		quoted := regexp.QuoteMeta(keyword)
		filter["$or"] = bson.A{
			bson.M{"packet_id": bson.M{"$regex": quoted}},
			bson.M{"grabber_id": bson.M{"$regex": quoted}},
		}
	}
	return mongoutil.FindPage[*chat.RedPacketGrab](ctx, o.coll, filter, p,
		options.Find().SetSort(bson.D{{Key: "create_time", Value: -1}}))
}
