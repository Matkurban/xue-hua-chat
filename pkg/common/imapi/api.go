package imapi

import (
	"github.com/openimsdk/protocol/auth"
	"github.com/openimsdk/protocol/group"
	"github.com/openimsdk/protocol/relation"
	"github.com/openimsdk/protocol/user"
)

// im caller.
var (
	getAdminToken = NewApiCaller[auth.GetAdminTokenReq, auth.GetAdminTokenResp]("/auth/get_admin_token")
	getuserToken  = NewApiCaller[auth.GetUserTokenReq, auth.GetUserTokenResp]("/auth/get_user_token")
	forceOffLine  = NewApiCaller[auth.ForceLogoutReq, auth.ForceLogoutResp]("/auth/force_logout")

	updateUserInfo            = NewApiCaller[user.UpdateUserInfoReq, user.UpdateUserInfoResp]("/user/update_user_info")
	registerUser              = NewApiCaller[user.UserRegisterReq, user.UserRegisterResp]("/user/user_register")
	getUserInfo               = NewApiCaller[user.GetDesignateUsersReq, user.GetDesignateUsersResp]("/user/get_users_info")
	getAllOnlineUsers         = NewApiCaller[user.GetAllOnlineUsersReq, user.GetAllOnlineUsersResp]("/user/get_all_online_users")
	accountCheck              = NewApiCaller[user.AccountCheckReq, user.AccountCheckResp]("/user/account_check")
	addNotificationAccount    = NewApiCaller[user.AddNotificationAccountReq, user.AddNotificationAccountResp]("/user/add_notification_account")
	updateNotificationAccount = NewApiCaller[user.UpdateNotificationAccountInfoReq, user.UpdateNotificationAccountInfoResp]("/user/update_notification_account")

	getGroupsInfo = NewApiCaller[group.GetGroupsInfoReq, group.GetGroupsInfoResp]("/group/get_groups_info")
	inviteToGroup = NewApiCaller[group.InviteUserToGroupReq, group.InviteUserToGroupResp]("/group/invite_user_to_group")

	registerUserCount = NewApiCaller[user.UserRegisterCountReq, user.UserRegisterCountResp]("/statistics/user/register")

	friendUserIDs = NewApiCaller[relation.GetFriendIDsReq, relation.GetFriendIDsResp]("/friend/get_friend_id")
	importFriend  = NewApiCaller[relation.ImportFriendReq, relation.ImportFriendResp]("/friend/import_friend")

	sendSimpleMsg            = NewApiCaller[SendSingleMsgReq, SendSingleMsgResp]("/msg/send_simple_msg")
	sendBusinessNotification = NewApiCaller[SendBusinessNotificationReq, SendBusinessNotificationResp]("/msg/send_business_notification")
	sendMsg                  = NewApiCaller[SendCustomMsgReq, SendCustomMsgResp]("/msg/send_msg")
	modifyMsgContent         = NewApiCaller[ModifyMsgContentReq, ModifyMsgContentResp]("/msg/modify_msg_content")
	batchSendMsg             = NewLongApiCaller[BatchSendMsgReq, BatchSendMsgResp]("/msg/batch_send_msg")
	batchRevokeMsg           = NewLongApiCaller[BatchRevokeMsgReq, BatchRevokeMsgResp]("/msg/batch_revoke_msg")
	getGroupMemberUserIDs    = NewApiCaller[GetGroupMemberUserIDsReq, GetGroupMemberUserIDsResp]("/group/get_group_member_user_id")
	getGroupMembersInfo      = NewApiCaller[group.GetGroupMembersInfoReq, group.GetGroupMembersInfoResp]("/group/get_group_members_info")

	createMoment  = NewApiCaller[CreateMomentReq, CreateMomentResp]("/moment/create")
	deleteMoment  = NewApiCaller[DeleteMomentReq, MomentMutationResp]("/moment/delete")
	listMoments   = NewApiCaller[ListMomentsReq, ListMomentsResp]("/moment/list")
	listTimeline  = NewApiCaller[ListTimelineReq, ListTimelineResp]("/moment/timeline")
	syncMoments   = NewApiCaller[SyncMomentsReq, SyncMomentsResp]("/moment/sync")
	commentMoment = NewApiCaller[CommentMomentReq, CommentMomentResp]("/moment/comment")
	deleteComment = NewApiCaller[DeleteCommentReq, DeleteCommentResp]("/moment/delete_comment")
	likeMoment    = NewApiCaller[LikeMomentReq, MomentMutationResp]("/moment/like")
	unlikeMoment  = NewApiCaller[UnlikeMomentReq, MomentMutationResp]("/moment/unlike")

	adminListMoments   = NewApiCaller[AdminListMomentsReq, AdminListMomentsResp]("/moment/admin/list")
	adminGetMoment     = NewApiCaller[AdminMomentIDReq, AdminMomentDetailResp]("/moment/admin/detail")
	adminDeleteMoments = NewApiCaller[AdminMomentIDsReq, AdminDeleteMomentsResp]("/moment/admin/delete")
	adminListComments  = NewApiCaller[AdminListCommentsReq, AdminListCommentsResp]("/moment/admin/comment_list")
	adminDeleteComment = NewApiCaller[AdminDeleteCommentReq, struct{}]("/moment/admin/delete_comment")
	adminListLikes     = NewApiCaller[AdminListLikesReq, AdminListLikesResp]("/moment/admin/like_list")
	adminBanMoments    = NewApiCaller[AdminMomentIDsReq, AdminSetStatusResp]("/moment/admin/ban")
	adminRestoreMoment = NewApiCaller[AdminMomentIDsReq, AdminSetStatusResp]("/moment/admin/restore")
	adminMomentStats   = NewApiCaller[AdminMomentStatsReq, AdminMomentStatsResp]("/moment/admin/stats")

	createFriendTag        = NewApiCaller[CreateFriendTagReq, CreateFriendTagResp]("/friend_tag/create")
	renameFriendTag        = NewApiCaller[RenameFriendTagReq, struct{}]("/friend_tag/rename")
	deleteFriendTag        = NewApiCaller[DeleteFriendTagReq, struct{}]("/friend_tag/delete")
	addFriendTagMembers    = NewApiCaller[AddFriendTagMembersReq, struct{}]("/friend_tag/add_members")
	removeFriendTagMembers = NewApiCaller[RemoveFriendTagMembersReq, struct{}]("/friend_tag/remove_members")
	listFriendTags         = NewApiCaller[ListFriendTagsReq, ListFriendTagsResp]("/friend_tag/list")
	listFriendTagMembers   = NewApiCaller[ListFriendTagMembersReq, ListFriendTagMembersResp]("/friend_tag/list_members")

	addFavorite    = NewApiCaller[AddFavoriteReq, AddFavoriteResp]("/favorite/add")
	removeFavorite = NewApiCaller[RemoveFavoriteReq, struct{}]("/favorite/remove")
	listFavorites  = NewApiCaller[ListFavoritesReq, ListFavoritesResp]("/favorite/list")
	updateFavorite = NewApiCaller[UpdateFavoriteReq, UpdateFavoriteResp]("/favorite/update")
)
