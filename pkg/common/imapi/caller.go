package imapi

import (
	"context"
	"sync"
	"time"

	"chat/pkg/botstruct"
	"chat/pkg/common/mctx"
	"chat/pkg/eerrs"
	"chat/pkg/common/errs"
	"chat/pkg/common/log"
	"github.com/openimsdk/protocol/auth"
	"github.com/openimsdk/protocol/constant"
	"github.com/openimsdk/protocol/group"
	"github.com/openimsdk/protocol/relation"
	"github.com/openimsdk/protocol/sdkws"
	"github.com/openimsdk/protocol/user"
)

type CallerInterface interface {
	ImAdminTokenWithDefaultAdmin(ctx context.Context) (string, error)
	ImportFriend(ctx context.Context, ownerUserID string, friendUserID []string) error
	GetUserToken(ctx context.Context, userID string, platform int32) (string, error)
	GetAdminTokenCache(ctx context.Context, userID string) (string, error)
	GetAdminTokenServer(ctx context.Context, userID string) (string, error)
	InviteToGroup(ctx context.Context, userID string, groupIDs []string) error

	UpdateUserInfo(ctx context.Context, userID string, nickName string, faceURL string) error
	GetUserInfo(ctx context.Context, userID string) (*sdkws.UserInfo, error)
	GetUsersInfo(ctx context.Context, userIDs []string) ([]*sdkws.UserInfo, error)
	AddNotificationAccount(ctx context.Context, req *user.AddNotificationAccountReq) error
	UpdateNotificationAccount(ctx context.Context, req *user.UpdateNotificationAccountInfoReq) error

	ForceOffLine(ctx context.Context, userID string) error
	RegisterUser(ctx context.Context, users []*sdkws.UserInfo) error
	FindGroupInfo(ctx context.Context, groupIDs []string) ([]*sdkws.GroupInfo, error)
	UserRegisterCount(ctx context.Context, start int64, end int64) (map[string]int64, int64, error)
	FriendUserIDs(ctx context.Context, userID string) ([]string, error)
	AccountCheckSingle(ctx context.Context, userID string) (bool, error)
	SendSimpleMsg(ctx context.Context, req *SendSingleMsgReq, key string) error
	SendBusinessNotification(ctx context.Context, key string, data string, sendUserID string, recvUserID string) error
	GetAllOnlineUserIDs(ctx context.Context) ([]string, error)
	SendCustomMessage(ctx context.Context, req *SendCustomMsgReq) error
	SendCustomMessageResp(ctx context.Context, req *SendCustomMsgReq) (*SendCustomMsgResp, error)
	ModifyMsgContent(ctx context.Context, conversationID, clientMsgID string, mergeContent map[string]any) error
	BatchSendMsg(ctx context.Context, req *BatchSendMsgReq) (*BatchSendMsgResp, error)
	BatchRevokeMsg(ctx context.Context, req *BatchRevokeMsgReq) (*BatchRevokeMsgResp, error)
	GetGroupMemberUserIDs(ctx context.Context, groupID string) ([]string, error)
	GetGroupMembersInfo(ctx context.Context, groupID string, userIDs []string) ([]*sdkws.GroupMemberFullInfo, error)

	CreateMoment(ctx context.Context, req *CreateMomentReq) (*CreateMomentResp, error)
	DeleteMoment(ctx context.Context, req *DeleteMomentReq) error
	ListMoments(ctx context.Context, req *ListMomentsReq) (*ListMomentsResp, error)
	CommentMoment(ctx context.Context, req *CommentMomentReq) (*CommentMomentResp, error)
	DeleteComment(ctx context.Context, req *DeleteCommentReq) error
	LikeMoment(ctx context.Context, req *LikeMomentReq) error
	UnlikeMoment(ctx context.Context, req *UnlikeMomentReq) error

	AdminListMoments(ctx context.Context, req *AdminListMomentsReq) (*AdminListMomentsResp, error)
	AdminGetMoment(ctx context.Context, req *AdminMomentIDReq) (*AdminMomentDetailResp, error)
	AdminDeleteMoments(ctx context.Context, req *AdminMomentIDsReq) (*AdminDeleteMomentsResp, error)
	AdminListMomentComments(ctx context.Context, req *AdminListCommentsReq) (*AdminListCommentsResp, error)
	AdminDeleteMomentComment(ctx context.Context, req *AdminDeleteCommentReq) error
	AdminListMomentLikes(ctx context.Context, req *AdminListLikesReq) (*AdminListLikesResp, error)
	AdminBanMoments(ctx context.Context, req *AdminMomentIDsReq) (*AdminSetStatusResp, error)
	AdminRestoreMoments(ctx context.Context, req *AdminMomentIDsReq) (*AdminSetStatusResp, error)
	AdminMomentStats(ctx context.Context) (*AdminMomentStatsResp, error)

	CreateFriendTag(ctx context.Context, req *CreateFriendTagReq) (*CreateFriendTagResp, error)
	RenameFriendTag(ctx context.Context, req *RenameFriendTagReq) error
	DeleteFriendTag(ctx context.Context, req *DeleteFriendTagReq) error
	AddFriendTagMembers(ctx context.Context, req *AddFriendTagMembersReq) error
	RemoveFriendTagMembers(ctx context.Context, req *RemoveFriendTagMembersReq) error
	ListFriendTags(ctx context.Context, req *ListFriendTagsReq) (*ListFriendTagsResp, error)
	ListFriendTagMembers(ctx context.Context, req *ListFriendTagMembersReq) (*ListFriendTagMembersResp, error)

	AddFavorite(ctx context.Context, req *AddFavoriteReq) (*AddFavoriteResp, error)
	RemoveFavorite(ctx context.Context, req *RemoveFavoriteReq) error
	ListFavorites(ctx context.Context, req *ListFavoritesReq) (*ListFavoritesResp, error)
	UpdateFavorite(ctx context.Context, req *UpdateFavoriteReq) (*UpdateFavoriteResp, error)
}

type authToken struct {
	token   string
	expired time.Time
}

type Caller struct {
	imApi           string
	imSecret        string
	defaultIMUserID string
	tokenCache      map[string]*authToken
	lock            sync.RWMutex
}

func New(imApi string, imSecret string, defaultIMUserID string) CallerInterface {
	return &Caller{
		imApi:           imApi,
		imSecret:        imSecret,
		defaultIMUserID: defaultIMUserID,
		tokenCache:      make(map[string]*authToken),
		lock:            sync.RWMutex{},
	}
}

func (c *Caller) ImportFriend(ctx context.Context, ownerUserID string, friendUserIDs []string) error {
	if len(friendUserIDs) == 0 {
		return nil
	}
	_, err := importFriend.Call(ctx, c.imApi, &relation.ImportFriendReq{
		OwnerUserID:   ownerUserID,
		FriendUserIDs: friendUserIDs,
	})
	return err
}

func (c *Caller) ImAdminTokenWithDefaultAdmin(ctx context.Context) (string, error) {
	return c.GetAdminTokenCache(ctx, c.defaultIMUserID)
}

func (c *Caller) GetAdminTokenCache(ctx context.Context, userID string) (string, error) {
	c.lock.RLock()
	t, ok := c.tokenCache[userID]
	c.lock.RUnlock()
	if ok && t.expired.After(time.Now()) {
		return t.token, nil
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	t, ok = c.tokenCache[userID]
	if ok && t.expired.After(time.Now()) {
		return t.token, nil
	}
	token, err := c.GetAdminTokenServer(ctx, userID)
	if err != nil {
		return "", err
	}
	c.tokenCache[userID] = &authToken{token: token, expired: time.Now().Add(time.Minute * 4)}
	return token, nil
}

func (c *Caller) GetAdminTokenServer(ctx context.Context, userID string) (string, error) {
	resp, err := getAdminToken.Call(ctx, c.imApi, &auth.GetAdminTokenReq{
		Secret: c.imSecret,
		UserID: userID,
	})
	if err != nil {
		return "", err
	}
	log.ZDebug(ctx, "get im admin token from server", "userID", userID, "token", resp.Token)
	return resp.Token, nil
}

func (c *Caller) GetUserToken(ctx context.Context, userID string, platformID int32) (string, error) {
	resp, err := getuserToken.Call(ctx, c.imApi, &auth.GetUserTokenReq{
		PlatformID: platformID,
		UserID:     userID,
	})
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

func (c *Caller) InviteToGroup(ctx context.Context, userID string, groupIDs []string) error {
	for _, groupID := range groupIDs {
		_, _ = inviteToGroup.Call(ctx, c.imApi, &group.InviteUserToGroupReq{
			GroupID:        groupID,
			Reason:         "",
			InvitedUserIDs: []string{userID},
		})
	}
	return nil
}

func (c *Caller) UpdateUserInfo(ctx context.Context, userID string, nickName string, faceURL string) error {
	_, err := updateUserInfo.Call(ctx, c.imApi, &user.UpdateUserInfoReq{UserInfo: &sdkws.UserInfo{
		UserID:   userID,
		Nickname: nickName,
		FaceURL:  faceURL,
	}})
	return err
}

func (c *Caller) GetUserInfo(ctx context.Context, userID string) (*sdkws.UserInfo, error) {
	resp, err := c.GetUsersInfo(ctx, []string{userID})
	if err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		return nil, errs.ErrRecordNotFound.WrapMsg("record not found")
	}
	return resp[0], nil
}

func (c *Caller) GetUsersInfo(ctx context.Context, userIDs []string) ([]*sdkws.UserInfo, error) {
	resp, err := getUserInfo.Call(ctx, c.imApi, &user.GetDesignateUsersReq{
		UserIDs: userIDs,
	})
	if err != nil {
		return nil, err
	}
	return resp.UsersInfo, nil
}

func (c *Caller) RegisterUser(ctx context.Context, users []*sdkws.UserInfo) error {
	_, err := registerUser.Call(ctx, c.imApi, &user.UserRegisterReq{
		Users: users,
	})
	return err
}

func (c *Caller) ForceOffLine(ctx context.Context, userID string) error {
	for id := range constant.PlatformID2Name {
		_, _ = forceOffLine.Call(ctx, c.imApi, &auth.ForceLogoutReq{
			PlatformID: int32(id),
			UserID:     userID,
		})
	}
	return nil
}

func (c *Caller) FindGroupInfo(ctx context.Context, groupIDs []string) ([]*sdkws.GroupInfo, error) {
	resp, err := getGroupsInfo.Call(ctx, c.imApi, &group.GetGroupsInfoReq{
		GroupIDs: groupIDs,
	})
	if err != nil {
		return nil, err
	}
	return resp.GroupInfos, nil
}

func (c *Caller) UserRegisterCount(ctx context.Context, start int64, end int64) (map[string]int64, int64, error) {
	resp, err := registerUserCount.Call(ctx, c.imApi, &user.UserRegisterCountReq{
		Start: start,
		End:   end,
	})
	if err != nil {
		return nil, 0, err
	}
	return resp.Count, resp.Total, nil
}

func (c *Caller) FriendUserIDs(ctx context.Context, userID string) ([]string, error) {
	resp, err := friendUserIDs.Call(ctx, c.imApi, &relation.GetFriendIDsReq{UserID: userID})
	if err != nil {
		return nil, err
	}
	return resp.FriendIDs, nil
}

// return true when isUserNotExist.
func (c *Caller) AccountCheckSingle(ctx context.Context, userID string) (bool, error) {
	resp, err := accountCheck.Call(ctx, c.imApi, &user.AccountCheckReq{CheckUserIDs: []string{userID}})
	if err != nil {
		return false, err
	}
	if resp.Results[0].AccountStatus == constant.Registered {
		return false, eerrs.ErrAccountAlreadyRegister.Wrap()
	}
	return true, nil
}

func (c *Caller) SendSimpleMsg(ctx context.Context, req *SendSingleMsgReq, key string) error {
	_, err := sendSimpleMsg.CallWithQuery(ctx, c.imApi, req, map[string]string{botstruct.Key: key})
	return err
}

func (c *Caller) SendBusinessNotification(ctx context.Context, key string, data string, sendUserID string, recvUserID string) error {
	_, err := sendBusinessNotification.Call(ctx, c.imApi, &SendBusinessNotificationReq{
		Key:              key,
		Data:             data,
		SendUserID:       sendUserID,
		RecvUserID:       recvUserID,
		SendMsg:          false,
		ReliabilityLevel: 1,
	})
	return err
}

func (c *Caller) GetAllOnlineUserIDs(ctx context.Context) ([]string, error) {
	if c == nil {
		return nil, errs.ErrInternalServer.WrapMsg("im caller is nil")
	}
	token, err := c.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		return nil, err
	}
	apiCtx := mctx.WithApiToken(ctx, token)
	const maxPages = 50
	var (
		cursor uint64
		ids    []string
	)
	seen := make(map[string]struct{})
	for page := 0; page < maxPages; page++ {
		resp, err := getAllOnlineUsers.Call(apiCtx, c.imApi, &user.GetAllOnlineUsersReq{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		if resp == nil {
			break
		}
		for _, st := range resp.GetStatusList() {
			if st == nil || st.UserID == "" {
				continue
			}
			if st.Status != constant.Online && len(st.PlatformIDs) == 0 {
				continue
			}
			if _, ok := seen[st.UserID]; ok {
				continue
			}
			seen[st.UserID] = struct{}{}
			ids = append(ids, st.UserID)
		}
		next := resp.GetNextCursor()
		if next == 0 || next == cursor {
			break
		}
		cursor = next
	}
	return ids, nil
}

func (c *Caller) SendCustomMessage(ctx context.Context, req *SendCustomMsgReq) error {
	_, err := sendMsg.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) SendCustomMessageResp(ctx context.Context, req *SendCustomMsgReq) (*SendCustomMsgResp, error) {
	return sendMsg.Call(ctx, c.imApi, req)
}

func (c *Caller) ModifyMsgContent(ctx context.Context, conversationID, clientMsgID string, mergeContent map[string]any) error {
	_, err := modifyMsgContent.Call(ctx, c.imApi, &ModifyMsgContentReq{
		ConversationID: conversationID,
		ClientMsgID:    clientMsgID,
		MergeContent:   mergeContent,
	})
	return err
}

func (c *Caller) BatchSendMsg(ctx context.Context, req *BatchSendMsgReq) (*BatchSendMsgResp, error) {
	if req == nil {
		return nil, errs.ErrArgs.WrapMsg("batch send req is nil")
	}
	if req.RecvIDs == nil {
		req.RecvIDs = []string{}
	}
	return batchSendMsg.Call(ctx, c.imApi, req)
}

func (c *Caller) BatchRevokeMsg(ctx context.Context, req *BatchRevokeMsgReq) (*BatchRevokeMsgResp, error) {
	if req == nil {
		return nil, errs.ErrArgs.WrapMsg("batch revoke req is nil")
	}
	return batchRevokeMsg.Call(ctx, c.imApi, req)
}

func (c *Caller) GetGroupMemberUserIDs(ctx context.Context, groupID string) ([]string, error) {
	resp, err := getGroupMemberUserIDs.Call(ctx, c.imApi, &GetGroupMemberUserIDsReq{GroupID: groupID})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return resp.UserIDs, nil
}

func (c *Caller) GetGroupMembersInfo(ctx context.Context, groupID string, userIDs []string) ([]*sdkws.GroupMemberFullInfo, error) {
	if groupID == "" || len(userIDs) == 0 {
		return nil, nil
	}
	resp, err := getGroupMembersInfo.Call(ctx, c.imApi, &group.GetGroupMembersInfoReq{
		GroupID: groupID,
		UserIDs: userIDs,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return resp.Members, nil
}

func (c *Caller) CreateMoment(ctx context.Context, req *CreateMomentReq) (*CreateMomentResp, error) {
	return createMoment.Call(ctx, c.imApi, req)
}

func (c *Caller) DeleteMoment(ctx context.Context, req *DeleteMomentReq) error {
	_, err := deleteMoment.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) ListMoments(ctx context.Context, req *ListMomentsReq) (*ListMomentsResp, error) {
	return listMoments.Call(ctx, c.imApi, req)
}

func (c *Caller) CommentMoment(ctx context.Context, req *CommentMomentReq) (*CommentMomentResp, error) {
	return commentMoment.Call(ctx, c.imApi, req)
}

func (c *Caller) DeleteComment(ctx context.Context, req *DeleteCommentReq) error {
	_, err := deleteComment.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) LikeMoment(ctx context.Context, req *LikeMomentReq) error {
	_, err := likeMoment.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) UnlikeMoment(ctx context.Context, req *UnlikeMomentReq) error {
	_, err := unlikeMoment.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) CreateFriendTag(ctx context.Context, req *CreateFriendTagReq) (*CreateFriendTagResp, error) {
	return createFriendTag.Call(ctx, c.imApi, req)
}

func (c *Caller) RenameFriendTag(ctx context.Context, req *RenameFriendTagReq) error {
	_, err := renameFriendTag.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) DeleteFriendTag(ctx context.Context, req *DeleteFriendTagReq) error {
	_, err := deleteFriendTag.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) AddFriendTagMembers(ctx context.Context, req *AddFriendTagMembersReq) error {
	_, err := addFriendTagMembers.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) RemoveFriendTagMembers(ctx context.Context, req *RemoveFriendTagMembersReq) error {
	_, err := removeFriendTagMembers.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) ListFriendTags(ctx context.Context, req *ListFriendTagsReq) (*ListFriendTagsResp, error) {
	return listFriendTags.Call(ctx, c.imApi, req)
}

func (c *Caller) ListFriendTagMembers(ctx context.Context, req *ListFriendTagMembersReq) (*ListFriendTagMembersResp, error) {
	return listFriendTagMembers.Call(ctx, c.imApi, req)
}

func (c *Caller) AddFavorite(ctx context.Context, req *AddFavoriteReq) (*AddFavoriteResp, error) {
	return addFavorite.Call(ctx, c.imApi, req)
}

func (c *Caller) RemoveFavorite(ctx context.Context, req *RemoveFavoriteReq) error {
	_, err := removeFavorite.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) ListFavorites(ctx context.Context, req *ListFavoritesReq) (*ListFavoritesResp, error) {
	return listFavorites.Call(ctx, c.imApi, req)
}

func (c *Caller) UpdateFavorite(ctx context.Context, req *UpdateFavoriteReq) (*UpdateFavoriteResp, error) {
	return updateFavorite.Call(ctx, c.imApi, req)
}

func (c *Caller) AddNotificationAccount(ctx context.Context, req *user.AddNotificationAccountReq) error {
	_, err := addNotificationAccount.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) UpdateNotificationAccount(ctx context.Context, req *user.UpdateNotificationAccountInfoReq) error {
	_, err := updateNotificationAccount.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) AdminListMoments(ctx context.Context, req *AdminListMomentsReq) (*AdminListMomentsResp, error) {
	return adminListMoments.Call(ctx, c.imApi, req)
}

func (c *Caller) AdminGetMoment(ctx context.Context, req *AdminMomentIDReq) (*AdminMomentDetailResp, error) {
	return adminGetMoment.Call(ctx, c.imApi, req)
}

func (c *Caller) AdminDeleteMoments(ctx context.Context, req *AdminMomentIDsReq) (*AdminDeleteMomentsResp, error) {
	return adminDeleteMoments.Call(ctx, c.imApi, req)
}

func (c *Caller) AdminListMomentComments(ctx context.Context, req *AdminListCommentsReq) (*AdminListCommentsResp, error) {
	return adminListComments.Call(ctx, c.imApi, req)
}

func (c *Caller) AdminDeleteMomentComment(ctx context.Context, req *AdminDeleteCommentReq) error {
	_, err := adminDeleteComment.Call(ctx, c.imApi, req)
	return err
}

func (c *Caller) AdminListMomentLikes(ctx context.Context, req *AdminListLikesReq) (*AdminListLikesResp, error) {
	return adminListLikes.Call(ctx, c.imApi, req)
}

func (c *Caller) AdminBanMoments(ctx context.Context, req *AdminMomentIDsReq) (*AdminSetStatusResp, error) {
	return adminBanMoments.Call(ctx, c.imApi, req)
}

func (c *Caller) AdminRestoreMoments(ctx context.Context, req *AdminMomentIDsReq) (*AdminSetStatusResp, error) {
	return adminRestoreMoment.Call(ctx, c.imApi, req)
}

func (c *Caller) AdminMomentStats(ctx context.Context) (*AdminMomentStatsResp, error) {
	return adminMomentStats.Call(ctx, c.imApi, &AdminMomentStatsReq{})
}
