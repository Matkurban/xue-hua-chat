package chat

import (
	"context"
	"time"

	"chat/pkg/common/statsday"
	"chat/pkg/protocol/chat"
	"gitee.com/matkurban/openimsdk-tools/errs"
)

// fillDays zero-fills Asia/Shanghai days; see statsday.FillDays.
func fillDays(startMs, endMs int64, src map[string]int64) (map[string]int64, error) {
	return statsday.FillDays(startMs, endMs, src)
}

// UserLoginCount: count = day -> distinct users who logged in that day (zero-filled);
// loginCount = distinct users who logged in during [start,end); unloginCount = registered users - loginCount.
func (o *chatSvr) UserLoginCount(ctx context.Context, req *chat.UserLoginCountReq) (*chat.UserLoginCountResp, error) {
	resp := &chat.UserLoginCountResp{}
	if req.Start > req.End {
		return nil, errs.ErrArgs.WrapMsg("start > end")
	}
	total, err := o.Database.NewUserCountTotal(ctx, nil)
	if err != nil {
		return nil, err
	}
	count, loginCount, err := o.Database.UserLoginCountRangeEverydayTotal(ctx, new(time.UnixMilli(req.Start)), new(time.UnixMilli(req.End)))
	if err != nil {
		return nil, err
	}
	filled, err := fillDays(req.Start, req.End, count)
	if err != nil {
		return nil, err
	}
	resp.LoginCount = loginCount
	resp.UnloginCount = max(total-loginCount, 0)
	resp.Count = filled
	return resp, nil
}
