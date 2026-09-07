package chat

import (
	"chat/pkg/protocol/chat"
	"context"
)

func (o *chatSvr) SetAllowRegister(ctx context.Context, req *chat.SetAllowRegisterReq) (*chat.SetAllowRegisterResp, error) {
	o.AllowRegister = req.AllowRegister
	return &chat.SetAllowRegisterResp{}, nil
}

func (o *chatSvr) GetAllowRegister(ctx context.Context, req *chat.GetAllowRegisterReq) (*chat.GetAllowRegisterResp, error) {
	return &chat.GetAllowRegisterResp{AllowRegister: o.AllowRegister}, nil
}
