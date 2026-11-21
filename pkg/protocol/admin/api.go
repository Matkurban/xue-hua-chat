package admin

import "chat/pkg/util/datautil"

func (x *GetClientConfigResp) ApiFormat() {
	datautil.InitMap(&x.Config)
}
