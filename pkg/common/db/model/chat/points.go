package chat

import (
	"context"
	"math"
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

func NewUserPoints(db *mongo.Database) (chat.UserPointsInterface, error) {
	coll := db.Collection("user_points")
	_, err := coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &UserPoints{coll: coll}, nil
}

type UserPoints struct {
	coll *mongo.Collection
}

func (o *UserPoints) Create(ctx context.Context, up *chat.UserPoints) error {
	return mongoutil.InsertMany(ctx, o.coll, []*chat.UserPoints{up})
}

func (o *UserPoints) GetOrCreate(ctx context.Context, userID string) (*chat.UserPoints, error) {
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)
	now := time.Now()
	result := o.coll.FindOneAndUpdate(ctx,
		bson.M{"user_id": userID},
		bson.M{
			"$setOnInsert": bson.M{
				"user_id":     userID,
				"balance":     float64(0),
				"create_time": now,
				"update_time": now,
			},
		},
		opts,
	)
	var up chat.UserPoints
	if err := result.Decode(&up); err != nil {
		return nil, errs.Wrap(err)
	}
	return &up, nil
}

func (o *UserPoints) GetBalance(ctx context.Context, userID string) (float64, error) {
	up, err := o.GetOrCreate(ctx, userID)
	if err != nil {
		return 0, err
	}
	return math.Round(up.Balance*100) / 100, nil
}

// IncrBalance 原子增减积分；debitCheck=true 时若余额不足返回错误
func (o *UserPoints) IncrBalance(ctx context.Context, userID string, amount float64, debitCheck bool) error {
	// 确保金额精度不超过两位小数，避免浮点累积误差
	amount = math.Round(amount*100) / 100
	filter := bson.M{"user_id": userID}
	if debitCheck && amount < 0 {
		// 只在余额充足时更新
		filter["balance"] = bson.M{"$gte": -amount}
	}

	res, err := o.coll.UpdateOne(ctx, filter,
		bson.M{
			"$inc": bson.M{"balance": amount},
			"$set": bson.M{"update_time": time.Now()},
		},
	)
	if err != nil {
		return errs.Wrap(err)
	}
	if res.MatchedCount == 0 {
		if debitCheck && amount < 0 {
			return errs.New("insufficient points balance")
		}
		// 账户不存在则初始化
		if _, err = o.GetOrCreate(ctx, userID); err != nil {
			return err
		}
		return o.IncrBalance(ctx, userID, amount, debitCheck)
	}
	return nil
}

func (o *UserPoints) AdminAdjust(ctx context.Context, userID string, amount float64) error {
	return o.IncrBalance(ctx, userID, amount, amount < 0)
}

func (o *UserPoints) SearchAll(ctx context.Context, keyword string, p pagination.Pagination) (int64, []*chat.UserPoints, error) {
	filter := bson.M{}
	if keyword != "" {
		filter["user_id"] = bson.M{"$regex": regexp.QuoteMeta(keyword)}
	}
	return mongoutil.FindPage[*chat.UserPoints](ctx, o.coll, filter, p,
		options.Find().SetSort(bson.D{{Key: "balance", Value: -1}}))
}

// ─── PointsTransaction ───────────────────────────────────────────────────────

func NewPointsTransaction(db *mongo.Database) (chat.PointsTransactionInterface, error) {
	coll := db.Collection("points_transactions")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "tx_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "user_id", Value: 1}}},
		{Keys: bson.D{{Key: "related_id", Value: 1}}},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &PointsTransaction{coll: coll}, nil
}

type PointsTransaction struct {
	coll *mongo.Collection
}

func (o *PointsTransaction) Create(ctx context.Context, tx *chat.PointsTransaction) error {
	return mongoutil.InsertMany(ctx, o.coll, []*chat.PointsTransaction{tx})
}

func (o *PointsTransaction) FindByUser(ctx context.Context, userID string, p pagination.Pagination) (int64, []*chat.PointsTransaction, error) {
	return mongoutil.FindPage[*chat.PointsTransaction](ctx, o.coll,
		bson.M{"user_id": userID}, p,
		options.Find().SetSort(bson.D{{Key: "create_time", Value: -1}}))
}

func (o *PointsTransaction) SearchAll(ctx context.Context, keyword string, p pagination.Pagination) (int64, []*chat.PointsTransaction, error) {
	filter := bson.M{}
	if keyword != "" {
		quoted := regexp.QuoteMeta(keyword)
		filter["$or"] = bson.A{
			bson.M{"user_id": bson.M{"$regex": quoted}},
			bson.M{"related_id": bson.M{"$regex": quoted}},
		}
	}
	return mongoutil.FindPage[*chat.PointsTransaction](ctx, o.coll, filter, p,
		options.Find().SetSort(bson.D{{Key: "create_time", Value: -1}}))
}

func (o *PointsTransaction) SearchFiltered(ctx context.Context, userID string, txType int32, p pagination.Pagination) (int64, []*chat.PointsTransaction, error) {
	filter := bson.M{}
	if userID != "" {
		filter["user_id"] = userID
	}
	if txType > 0 {
		filter["tx_type"] = txType
	}
	return mongoutil.FindPage[*chat.PointsTransaction](ctx, o.coll, filter, p,
		options.Find().SetSort(bson.D{{Key: "create_time", Value: -1}}))
}

// SearchUserFiltered 用户端带筛选查询：txType、时间范围、关键字（匹配 relatedID/remark）
func (o *PointsTransaction) SearchUserFiltered(ctx context.Context, userID string, txType int32, startTime time.Time, endTime time.Time, keyword string, p pagination.Pagination) (int64, []*chat.PointsTransaction, error) {
	filter := bson.M{"user_id": userID}
	if txType > 0 {
		filter["tx_type"] = txType
	}
	timeFilter := bson.M{}
	if !startTime.IsZero() {
		timeFilter["$gte"] = startTime
	}
	if !endTime.IsZero() {
		timeFilter["$lte"] = endTime
	}
	if len(timeFilter) > 0 {
		filter["create_time"] = timeFilter
	}
	if keyword != "" {
		quoted := regexp.QuoteMeta(keyword)
		filter["$or"] = bson.A{
			bson.M{"related_id": bson.M{"$regex": quoted}},
			bson.M{"remark": bson.M{"$regex": quoted}},
		}
	}
	return mongoutil.FindPage[*chat.PointsTransaction](ctx, o.coll, filter, p,
		options.Find().SetSort(bson.D{{Key: "create_time", Value: -1}}))
}
