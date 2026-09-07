package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"chat/pkg/common/constant"
	"chat/pkg/eerrs"
	"chat/pkg/protocol/chat"
	"gitee.com/matkurban/openimsdk-tools/errs"
	constantpb "github.com/openimsdk/protocol/constant"
)

type CallbackBeforeAddFriendReq struct {
	CallbackCommand `json:"callbackCommand"`
	FromUserID      string `json:"fromUserID" `
	ToUserID        string `json:"toUserID"`
	ReqMsg          string `json:"reqMsg"`
	OperationID     string `json:"operationID"`
}

type CallbackCommand string

func (c CallbackCommand) GetCallbackCommand() string {
	return string(c)
}

func (o *chatSvr) ServerCallback(ctx context.Context, req *chat.ServerCallbackReq) (*chat.ServerCallbackResp, error) {
	switch req.Command {
	case constantpb.CallbackBeforeAddFriendCommand:
		var data CallbackBeforeAddFriendReq
		if err := json.Unmarshal([]byte(req.Body), &data); err != nil {
			return nil, errs.Wrap(err)
		}
		user, err := o.Database.TakeAttributeByUserID(ctx, data.ToUserID)
		if err != nil {
			return nil, err
		}
		if user.AllowAddFriend != constant.OrdinaryUserAddFriendEnable {
			return nil, eerrs.ErrRefuseFriend.WrapMsg(fmt.Sprintf("state %d", user.AllowAddFriend))
		}
		return &chat.ServerCallbackResp{}, nil
	default:
		return nil, errs.ErrArgs.WrapMsg(fmt.Sprintf("invalid command %s", req.Command))
	}
}
