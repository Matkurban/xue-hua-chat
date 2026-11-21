package chat

import (
	"context"
	"time"

	"chat/pkg/common/db/mongoutil"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"chat/pkg/common/db/table/chat"
)

func NewUserLoginRecord(db *mongo.Database) (chat.UserLoginRecordInterface, error) {
	coll := db.Collection("user_login_record")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "login_time", Value: -1},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &UserLoginRecord{
		coll: coll,
	}, nil
}

type UserLoginRecord struct {
	coll *mongo.Collection
}

func (o *UserLoginRecord) Create(ctx context.Context, records ...*chat.UserLoginRecord) error {
	return mongoutil.InsertMany(ctx, o.coll, records)
}

func (o *UserLoginRecord) CountTotal(ctx context.Context, before *time.Time) (count int64, err error) {
	filter := bson.M{}
	if before != nil {
		filter["login_time"] = bson.M{"$lt": before}
	}
	return mongoutil.Count(ctx, o.coll, filter)
}

// StatsTimezone is the offset used to bucket statistics by calendar day (Asia/Shanghai, no DST).
// A fixed offset string is used so MongoDB needs no tz database.
const StatsTimezone = "+08:00"

// CountRangeEverydayTotal returns day(YYYY-MM-DD, Asia/Shanghai) -> distinct logged-in users that day,
// and the number of distinct users that logged in anywhere in [start, end).
// Days without any login are absent from the map (the caller zero-fills).
func (o *UserLoginRecord) CountRangeEverydayTotal(ctx context.Context, start *time.Time, end *time.Time) (map[string]int64, int64, error) {
	match := bson.M{}
	if start != nil || end != nil {
		filter := bson.M{}
		if start != nil {
			filter["$gte"] = *start
		}
		if end != nil {
			filter["$lt"] = *end
		}
		match["login_time"] = filter
	}
	pipeline := []bson.M{
		{"$match": match},
		{"$group": bson.M{"_id": bson.M{
			"user_id": "$user_id",
			"day":     bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$login_time", "timezone": StatsTimezone}},
		}}},
		{"$group": bson.M{"_id": "$_id.day", "count": bson.M{"$sum": 1}}},
	}
	type Temp struct {
		ID    string `bson:"_id"`
		Count int64  `bson:"count"`
	}
	res, err := mongoutil.Aggregate[Temp](ctx, o.coll, pipeline)
	if err != nil {
		return nil, 0, err
	}
	countMap := make(map[string]int64, len(res))
	for _, r := range res {
		countMap[r.ID] = r.Count
	}
	distinct, err := mongoutil.Aggregate[struct {
		N int64 `bson:"n"`
	}](ctx, o.coll, []bson.M{
		{"$match": match},
		{"$group": bson.M{"_id": "$user_id"}},
		{"$count": "n"},
	})
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if len(distinct) > 0 {
		total = distinct[0].N
	}
	return countMap, total, nil
}

func (o *UserLoginRecord) FindLastLoginTimes(ctx context.Context, userIDs []string) (map[string]time.Time, error) {
	res := make(map[string]time.Time, len(userIDs))
	if len(userIDs) == 0 {
		return res, nil
	}
	pipeline := []bson.M{
		{"$match": bson.M{"user_id": bson.M{"$in": userIDs}}},
		{"$group": bson.M{"_id": "$user_id", "last": bson.M{"$max": "$login_time"}}},
	}
	type temp struct {
		ID   string    `bson:"_id"`
		Last time.Time `bson:"last"`
	}
	list, err := mongoutil.Aggregate[temp](ctx, o.coll, pipeline)
	if err != nil {
		return nil, err
	}
	for _, v := range list {
		res[v.ID] = v.Last
	}
	return res, nil
}
