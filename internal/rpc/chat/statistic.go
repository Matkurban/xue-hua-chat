package chat

import (
	"context"
	"time"

	"chat/pkg/protocol/chat"
	"gitee.com/matkurban/openimsdk-tools/errs"
)

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
	resp.LoginCount = loginCount
	resp.UnloginCount = total - loginCount
	resp.Count = count
	return resp, nil
}
