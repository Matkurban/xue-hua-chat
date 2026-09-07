package chat

import (
	"chat/pkg/common/mctx"
	"chat/pkg/common/rtc"
	"chat/pkg/protocol/chat"
	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"github.com/gin-gonic/gin"
)

// ConferenceAdminApi exposes admin-only conference management endpoints.
type ConferenceAdminApi struct {
	Manager *rtc.ConferenceManager
}

func NewConferenceAdminApi(mgr *rtc.ConferenceManager) *ConferenceAdminApi {
	return &ConferenceAdminApi{Manager: mgr}
}

func (o *ConferenceAdminApi) mgrOrErr(c *gin.Context) bool {
	if o == nil || o.Manager == nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg("ConferenceManager is not configured"))
		return false
	}
	return true
}

// AdminListActiveConferences POST /admin/conference/list_active
func (o *ConferenceAdminApi) AdminListActiveConferences(c *gin.Context) {
	if _, err := mctx.CheckAdmin(c); err != nil {
		apiresp.GinError(c, err)
		return
	}
	var req chat.AdminListActiveConferencesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if !o.mgrOrErr(c) {
		return
	}
	pageNumber, showNumber := req.NormalizePage()
	total, list, err := o.Manager.ListActiveConferences(c.Request.Context(), pageNumber, showNumber)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	items := make([]*chat.AdminActiveConferenceItem, 0, len(list))
	for _, s := range list {
		if s == nil {
			continue
		}
		items = append(items, &chat.AdminActiveConferenceItem{
			RoomID:               s.RoomID,
			Title:                s.Title,
			AdminUserID:          s.AdminUserID,
			CreatorUserID:        s.CreatorUserID,
			CallType:             s.CallType,
			GroupID:              s.GroupID,
			CreateTime:           s.CreateTime,
			RealParticipantCount: s.RealParticipantCount,
			DisplayBoost:         s.DisplayBoost,
			ParticipantCount:     s.ParticipantCount,
			Locked:               s.Locked,
			Rev:                  s.Rev,
		})
	}
	apiresp.GinSuccess(c, &chat.AdminListActiveConferencesResp{
		Total:       total,
		Conferences: items,
	})
}

// AdminAdjustConferenceDisplayCount POST /admin/conference/adjust_display_count
func (o *ConferenceAdminApi) AdminAdjustConferenceDisplayCount(c *gin.Context) {
	if _, err := mctx.CheckAdmin(c); err != nil {
		apiresp.GinError(c, err)
		return
	}
	var req chat.AdminAdjustConferenceDisplayCountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.mgrOrErr(c) {
		return
	}
	summary, err := o.Manager.AdjustDisplayBoost(c, req.RoomID, req.AddCount)
	if err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, &chat.AdminAdjustConferenceDisplayCountResp{
		RoomID:               summary.RoomID,
		RealParticipantCount: summary.RealParticipantCount,
		DisplayBoost:         summary.DisplayBoost,
		ParticipantCount:     summary.ParticipantCount,
		Rev:                  summary.Rev,
		NotifiedCount:        summary.NotifiedCount,
		NotifyFailed:         summary.NotifyFailed,
	})
}

// AdminForceEndConference POST /admin/conference/force_end
func (o *ConferenceAdminApi) AdminForceEndConference(c *gin.Context) {
	if _, err := mctx.CheckAdmin(c); err != nil {
		apiresp.GinError(c, err)
		return
	}
	var req chat.AdminForceEndConferenceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg(err.Error()))
		return
	}
	if err := req.Check(); err != nil {
		apiresp.GinError(c, err)
		return
	}
	if !o.mgrOrErr(c) {
		return
	}
	if err := o.Manager.AdminForceEndConference(req.RoomID); err != nil {
		apiresp.GinError(c, errs.ErrInternalServer.WrapMsg(err.Error()))
		return
	}
	apiresp.GinSuccess(c, &chat.AdminForceEndConferenceResp{RoomID: req.RoomID})
}
