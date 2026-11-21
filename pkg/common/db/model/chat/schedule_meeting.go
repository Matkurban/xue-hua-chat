package chat

import (
	"context"
	"time"

	"chat/pkg/common/db/mongoutil"
	"chat/pkg/common/db/pagination"
	"chat/pkg/common/errs"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"chat/pkg/common/db/table/chat"
)

func NewScheduleMeetingSeries(db *mongo.Database) (chat.ScheduleMeetingSeriesInterface, error) {
	coll := db.Collection((chat.ScheduleMeetingSeries{}).TableName())
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "meeting_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "host_user_id", Value: 1}, {Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "invitee_user_ids", Value: 1}}},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &scheduleMeetingSeries{coll: coll}, nil
}

type scheduleMeetingSeries struct {
	coll *mongo.Collection
}

func (o *scheduleMeetingSeries) Create(ctx context.Context, series *chat.ScheduleMeetingSeries) error {
	return mongoutil.InsertMany(ctx, o.coll, []*chat.ScheduleMeetingSeries{series})
}

func (o *scheduleMeetingSeries) Take(ctx context.Context, id string) (*chat.ScheduleMeetingSeries, error) {
	return mongoutil.FindOne[*chat.ScheduleMeetingSeries](ctx, o.coll, bson.M{"_id": id})
}

func (o *scheduleMeetingSeries) TakeByMeetingID(ctx context.Context, meetingID string) (*chat.ScheduleMeetingSeries, error) {
	return mongoutil.FindOne[*chat.ScheduleMeetingSeries](ctx, o.coll, bson.M{"meeting_id": meetingID})
}

func (o *scheduleMeetingSeries) Update(ctx context.Context, series *chat.ScheduleMeetingSeries) error {
	series.UpdateTime = time.Now()
	set := bson.M{
		"meeting_id":                series.MeetingID,
		"host_user_id":              series.HostUserID,
		"title":                     series.Title,
		"call_type":                 series.CallType,
		"timezone":                  series.Timezone,
		"duration_minutes":          series.DurationMinutes,
		"enable_waiting_room":       series.EnableWaitingRoom,
		"mute_on_join":              series.MuteOnJoin,
		"allow_join_before_minutes": series.AllowJoinBeforeMinutes,
		"password_hash":             series.PasswordHash,
		"invitee_user_ids":          series.InviteeUserIDs,
		"source_group_ids":          series.SourceGroupIDs,
		"source_conversation_ids":   series.SourceConversationIDs,
		"remind_before_minutes":     series.RemindBeforeMinutes,
		"recurrence":                series.Recurrence,
		"status":                    series.Status,
		"update_time":               series.UpdateTime,
	}
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"_id": series.ID}, bson.M{"$set": set}, false)
}

func (o *scheduleMeetingSeries) Delete(ctx context.Context, id string) error {
	_, err := o.coll.DeleteOne(ctx, bson.M{"_id": id})
	return errs.Wrap(err)
}

func (o *scheduleMeetingSeries) ExistsMeetingID(ctx context.Context, meetingID string) (bool, error) {
	n, err := o.coll.CountDocuments(ctx, bson.M{"meeting_id": meetingID})
	if err != nil {
		return false, errs.Wrap(err)
	}
	return n > 0, nil
}

func (o *scheduleMeetingSeries) FindIDsByHost(ctx context.Context, hostUserID string) ([]string, error) {
	list, err := mongoutil.Find[*chat.ScheduleMeetingSeries](ctx, o.coll, bson.M{
		"host_user_id": hostUserID,
		"status":       chat.ScheduleSeriesStatusActive,
	}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(list))
	for _, s := range list {
		ids = append(ids, s.ID)
	}
	return ids, nil
}

func (o *scheduleMeetingSeries) FindIDsByInvitee(ctx context.Context, userID string) ([]string, error) {
	list, err := mongoutil.Find[*chat.ScheduleMeetingSeries](ctx, o.coll, bson.M{
		"invitee_user_ids": userID,
		"status":           chat.ScheduleSeriesStatusActive,
	}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(list))
	for _, s := range list {
		ids = append(ids, s.ID)
	}
	return ids, nil
}

func NewScheduleMeetingOccurrence(db *mongo.Database) (chat.ScheduleMeetingOccurrenceInterface, error) {
	coll := db.Collection((chat.ScheduleMeetingOccurrence{}).TableName())
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "meeting_id", Value: 1}, {Key: "start_time", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "start_time", Value: 1}}},
		{
			Keys:    bson.D{{Key: "series_id", Value: 1}, {Key: "start_time", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "override.invitee_user_ids", Value: 1}}},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &scheduleMeetingOccurrence{coll: coll}, nil
}

type scheduleMeetingOccurrence struct {
	coll *mongo.Collection
}

func (o *scheduleMeetingOccurrence) CreateMany(ctx context.Context, list []*chat.ScheduleMeetingOccurrence) error {
	if len(list) == 0 {
		return nil
	}
	return mongoutil.InsertMany(ctx, o.coll, list)
}

func (o *scheduleMeetingOccurrence) Take(ctx context.Context, id string) (*chat.ScheduleMeetingOccurrence, error) {
	return mongoutil.FindOne[*chat.ScheduleMeetingOccurrence](ctx, o.coll, bson.M{"_id": id})
}

func (o *scheduleMeetingOccurrence) Update(ctx context.Context, occ *chat.ScheduleMeetingOccurrence) error {
	occ.UpdateTime = time.Now()
	set := bson.M{
		"series_id":           occ.SeriesID,
		"meeting_id":          occ.MeetingID,
		"start_time":          occ.StartTime,
		"end_time":            occ.EndTime,
		"original_start_time": occ.OriginalStartTime,
		"status":              occ.Status,
		"ended_at":            occ.EndedAt,
		"ended_by_user_id":    occ.EndedByUserID,
		"end_reason":          occ.EndReason,
		"override":            occ.Override,
		"activated_room":      occ.ActivatedRoom,
		"update_time":         occ.UpdateTime,
	}
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"_id": occ.ID}, bson.M{"$set": set}, false)
}

func (o *scheduleMeetingOccurrence) FindSeriesIDsByOverrideInvitee(ctx context.Context, userID string) ([]string, error) {
	list, err := mongoutil.Find[*chat.ScheduleMeetingOccurrence](ctx, o.coll, bson.M{
		"override.invitee_user_ids": userID,
		"status": bson.M{"$in": []string{
			chat.ScheduleOccurrenceStatusScheduled,
			chat.ScheduleOccurrenceStatusOngoing,
		}},
	}, options.Find().SetProjection(bson.M{"series_id": 1}))
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(list))
	for _, o := range list {
		if _, ok := seen[o.SeriesID]; ok {
			continue
		}
		seen[o.SeriesID] = struct{}{}
		ids = append(ids, o.SeriesID)
	}
	return ids, nil
}

func (o *scheduleMeetingOccurrence) DeleteBySeries(ctx context.Context, seriesID string) error {
	_, err := o.coll.DeleteMany(ctx, bson.M{"series_id": seriesID})
	return errs.Wrap(err)
}

func (o *scheduleMeetingOccurrence) FindBySeries(ctx context.Context, seriesID string) ([]*chat.ScheduleMeetingOccurrence, error) {
	return mongoutil.Find[*chat.ScheduleMeetingOccurrence](ctx, o.coll,
		bson.M{"series_id": seriesID},
		options.Find().SetSort(bson.D{{Key: "start_time", Value: 1}}))
}

func (o *scheduleMeetingOccurrence) FindByMeetingID(ctx context.Context, meetingID string) ([]*chat.ScheduleMeetingOccurrence, error) {
	return mongoutil.Find[*chat.ScheduleMeetingOccurrence](ctx, o.coll,
		bson.M{"meeting_id": meetingID},
		options.Find().SetSort(bson.D{{Key: "start_time", Value: 1}}))
}

func (o *scheduleMeetingOccurrence) DeleteFutureScheduled(ctx context.Context, seriesID string, fromStartTime int64) (int64, error) {
	res, err := o.coll.DeleteMany(ctx, bson.M{
		"series_id":  seriesID,
		"status":     chat.ScheduleOccurrenceStatusScheduled,
		"start_time": bson.M{"$gte": fromStartTime},
	})
	if err != nil {
		return 0, errs.Wrap(err)
	}
	return res.DeletedCount, nil
}

func (o *scheduleMeetingOccurrence) CancelFrom(ctx context.Context, seriesID string, fromStartTime int64, includeThis bool) (int64, error) {
	op := "$gt"
	if includeThis {
		op = "$gte"
	}
	res, err := o.coll.UpdateMany(ctx, bson.M{
		"series_id":  seriesID,
		"status":     chat.ScheduleOccurrenceStatusScheduled,
		"start_time": bson.M{op: fromStartTime},
	}, bson.M{"$set": bson.M{
		"status":      chat.ScheduleOccurrenceStatusCancelled,
		"update_time": time.Now(),
	}})
	if err != nil {
		return 0, errs.Wrap(err)
	}
	return res.ModifiedCount, nil
}

func (o *scheduleMeetingOccurrence) ListForUser(
	ctx context.Context, userID string, hostSeriesIDs []string, role string, fromTime, toTime int64, p pagination.Pagination,
) (int64, []*chat.ScheduleMeetingOccurrence, error) {
	filter := bson.M{}
	and := bson.A{}
	if fromTime > 0 || toTime > 0 {
		st := bson.M{}
		if fromTime > 0 {
			st["$gte"] = fromTime
		}
		if toTime > 0 {
			st["$lte"] = toTime
		}
		and = append(and, bson.M{"start_time": st})
	}
	switch role {
	case "host":
		if len(hostSeriesIDs) == 0 {
			return 0, nil, nil
		}
		and = append(and, bson.M{"series_id": bson.M{"$in": hostSeriesIDs}})
	case "invited":
		// filled by caller via series invitee filter — here we use meeting list from invited series
		if len(hostSeriesIDs) == 0 {
			return 0, nil, nil
		}
		and = append(and, bson.M{"series_id": bson.M{"$in": hostSeriesIDs}})
	default: // all
		ors := bson.A{}
		if len(hostSeriesIDs) > 0 {
			ors = append(ors, bson.M{"series_id": bson.M{"$in": hostSeriesIDs}})
		}
		if len(ors) == 0 {
			return 0, nil, nil
		}
		and = append(and, bson.M{"$or": ors})
	}
	if len(and) > 0 {
		filter["$and"] = and
	}
	_ = userID
	return mongoutil.FindPage[*chat.ScheduleMeetingOccurrence](ctx, o.coll, filter, p,
		options.Find().SetSort(bson.D{{Key: "start_time", Value: 1}}))
}

func (o *scheduleMeetingOccurrence) FindJoinable(ctx context.Context, meetingID string, nowMs int64) (*chat.ScheduleMeetingOccurrence, error) {
	// Prefer ongoing, then scheduled around now.
	list, err := mongoutil.Find[*chat.ScheduleMeetingOccurrence](ctx, o.coll, bson.M{
		"meeting_id": meetingID,
		"status":     chat.ScheduleOccurrenceStatusOngoing,
	}, options.Find().SetLimit(1).SetSort(bson.D{{Key: "start_time", Value: -1}}))
	if err != nil {
		return nil, err
	}
	if len(list) > 0 {
		return list[0], nil
	}
	list, err = mongoutil.Find[*chat.ScheduleMeetingOccurrence](ctx, o.coll, bson.M{
		"meeting_id": meetingID,
		"status":     chat.ScheduleOccurrenceStatusScheduled,
		"start_time": bson.M{"$lte": nowMs + 24*60*60*1000}, // within next day window; finer checks in service
		"end_time":   bson.M{"$gte": nowMs - 60*60*1000},
	}, options.Find().SetLimit(5).SetSort(bson.D{{Key: "start_time", Value: 1}}))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, mongo.ErrNoDocuments
	}
	return list[0], nil
}
