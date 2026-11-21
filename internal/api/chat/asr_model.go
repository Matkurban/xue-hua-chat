package chat

import (
	admindb "chat/pkg/common/db/table/admin"
	"chat/pkg/common/apiresp"
	"github.com/gin-gonic/gin"
)

type AsrModelApi struct {
	store admindb.AsrModelInterface
}

func NewAsrModelApi(store admindb.AsrModelInterface) *AsrModelApi {
	return &AsrModelApi{store: store}
}

type asrModelListResp struct {
	List []*admindb.AsrModel `json:"list"`
}

func (o *AsrModelApi) List(c *gin.Context) {
	if o == nil || o.store == nil {
		apiresp.GinSuccess(c, &asrModelListResp{List: []*admindb.AsrModel{}})
		return
	}
	list, err := o.store.List(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if list == nil {
		list = []*admindb.AsrModel{}
	}
	apiresp.GinSuccess(c, &asrModelListResp{List: list})
}
