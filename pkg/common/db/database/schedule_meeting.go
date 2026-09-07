package database

import (
	"context"

	"gitee.com/matkurban/openimsdk-tools/db/mongoutil"
	"gitee.com/matkurban/openimsdk-tools/db/pagination"

	chatmodel "chat/pkg/common/db/model/chat"
	chatdb "chat/pkg/common/db/table/chat"
)

// ScheduleMeetingDatabaseInterface 预约会议持久化
type ScheduleMeetingDatabaseInterface interface {
	CreateSeries(ctx context.Context, series *chatdb.ScheduleMeetingSeries) error
	GetSeries(ctx context.Context, id string) (*chatdb.ScheduleMeetingSeries, error)
	GetSeriesByMeetingID(ctx context.Context, meetingID string) (*chatdb.ScheduleMeetingSeries, error)
	UpdateSeries(ctx context.Context, series *chatdb.ScheduleMeetingSeries) error
	ExistsMeetingID(ctx context.Context, meetingID string) (bool, error)
	FindSeriesIDsByHost(ctx context.Context, hostUserID string) ([]string, error)
	FindSeriesIDsByInvitee(ctx context.Context, userID string) ([]string, error)

	CreateOccurrences(ctx context.Context, list []*chatdb.ScheduleMeetingOccurrence) error
	GetOccurrence(ctx context.Context, id string) (*chatdb.ScheduleMeetingOccurrence, error)
	UpdateOccurrence(ctx context.Context, occ *chatdb.ScheduleMeetingOccurrence) error
	FindOccurrencesBySeries(ctx context.Context, seriesID string) ([]*chatdb.ScheduleMeetingOccurrence, error)
	FindOccurrencesByMeetingID(ctx context.Context, meetingID string) ([]*chatdb.ScheduleMeetingOccurrence, error)
	DeleteFutureScheduled(ctx context.Context, seriesID string, fromStartTime int64) (int64, error)
	CancelFrom(ctx context.Context, seriesID string, fromStartTime int64, includeThis bool) (int64, error)
	ListOccurrences(ctx context.Context, userID string, hostSeriesIDs []string, role string, fromTime, toTime int64, p pagination.Pagination) (int64, []*chatdb.ScheduleMeetingOccurrence, error)
	FindJoinableOccurrence(ctx context.Context, meetingID string, nowMs int64) (*chatdb.ScheduleMeetingOccurrence, error)
	FindSeriesIDsByOverrideInvitee(ctx context.Context, userID string) ([]string, error)
	DeleteSeries(ctx context.Context, seriesID string) error
}

type scheduleMeetingDatabase struct {
	series chatdb.ScheduleMeetingSeriesInterface
	occ    chatdb.ScheduleMeetingOccurrenceInterface
}

func NewScheduleMeetingDatabase(cli *mongoutil.Client) (ScheduleMeetingDatabaseInterface, error) {
	series, err := chatmodel.NewScheduleMeetingSeries(cli.GetDB())
	if err != nil {
		return nil, err
	}
	occ, err := chatmodel.NewScheduleMeetingOccurrence(cli.GetDB())
	if err != nil {
		return nil, err
	}
	return &scheduleMeetingDatabase{series: series, occ: occ}, nil
}

func (d *scheduleMeetingDatabase) CreateSeries(ctx context.Context, series *chatdb.ScheduleMeetingSeries) error {
	return d.series.Create(ctx, series)
}
func (d *scheduleMeetingDatabase) GetSeries(ctx context.Context, id string) (*chatdb.ScheduleMeetingSeries, error) {
	return d.series.Take(ctx, id)
}
func (d *scheduleMeetingDatabase) GetSeriesByMeetingID(ctx context.Context, meetingID string) (*chatdb.ScheduleMeetingSeries, error) {
	return d.series.TakeByMeetingID(ctx, meetingID)
}
func (d *scheduleMeetingDatabase) UpdateSeries(ctx context.Context, series *chatdb.ScheduleMeetingSeries) error {
	return d.series.Update(ctx, series)
}
func (d *scheduleMeetingDatabase) ExistsMeetingID(ctx context.Context, meetingID string) (bool, error) {
	return d.series.ExistsMeetingID(ctx, meetingID)
}
func (d *scheduleMeetingDatabase) FindSeriesIDsByHost(ctx context.Context, hostUserID string) ([]string, error) {
	return d.series.FindIDsByHost(ctx, hostUserID)
}
func (d *scheduleMeetingDatabase) FindSeriesIDsByInvitee(ctx context.Context, userID string) ([]string, error) {
	return d.series.FindIDsByInvitee(ctx, userID)
}
func (d *scheduleMeetingDatabase) CreateOccurrences(ctx context.Context, list []*chatdb.ScheduleMeetingOccurrence) error {
	return d.occ.CreateMany(ctx, list)
}
func (d *scheduleMeetingDatabase) GetOccurrence(ctx context.Context, id string) (*chatdb.ScheduleMeetingOccurrence, error) {
	return d.occ.Take(ctx, id)
}
func (d *scheduleMeetingDatabase) UpdateOccurrence(ctx context.Context, occ *chatdb.ScheduleMeetingOccurrence) error {
	return d.occ.Update(ctx, occ)
}
func (d *scheduleMeetingDatabase) FindOccurrencesBySeries(ctx context.Context, seriesID string) ([]*chatdb.ScheduleMeetingOccurrence, error) {
	return d.occ.FindBySeries(ctx, seriesID)
}
func (d *scheduleMeetingDatabase) FindOccurrencesByMeetingID(ctx context.Context, meetingID string) ([]*chatdb.ScheduleMeetingOccurrence, error) {
	return d.occ.FindByMeetingID(ctx, meetingID)
}
func (d *scheduleMeetingDatabase) DeleteFutureScheduled(ctx context.Context, seriesID string, fromStartTime int64) (int64, error) {
	return d.occ.DeleteFutureScheduled(ctx, seriesID, fromStartTime)
}
func (d *scheduleMeetingDatabase) CancelFrom(ctx context.Context, seriesID string, fromStartTime int64, includeThis bool) (int64, error) {
	return d.occ.CancelFrom(ctx, seriesID, fromStartTime, includeThis)
}
func (d *scheduleMeetingDatabase) ListOccurrences(ctx context.Context, userID string, hostSeriesIDs []string, role string, fromTime, toTime int64, p pagination.Pagination) (int64, []*chatdb.ScheduleMeetingOccurrence, error) {
	return d.occ.ListForUser(ctx, userID, hostSeriesIDs, role, fromTime, toTime, p)
}
func (d *scheduleMeetingDatabase) FindJoinableOccurrence(ctx context.Context, meetingID string, nowMs int64) (*chatdb.ScheduleMeetingOccurrence, error) {
	return d.occ.FindJoinable(ctx, meetingID, nowMs)
}
func (d *scheduleMeetingDatabase) FindSeriesIDsByOverrideInvitee(ctx context.Context, userID string) ([]string, error) {
	return d.occ.FindSeriesIDsByOverrideInvitee(ctx, userID)
}
func (d *scheduleMeetingDatabase) DeleteSeries(ctx context.Context, seriesID string) error {
	_ = d.occ.DeleteBySeries(ctx, seriesID)
	return d.series.Delete(ctx, seriesID)
}
