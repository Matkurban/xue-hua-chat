package admin

import "gitee.com/matkurban/openimsdk-tools/utils/datautil"

func (x *GetClientConfigResp) ApiFormat() {
	datautil.InitMap(&x.Config)
}
