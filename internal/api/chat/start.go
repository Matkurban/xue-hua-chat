package chat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	chatmw "chat/internal/api/mw"
	"chat/internal/api/util"
	"chat/pkg/common/config"
	rpCache "chat/pkg/common/db/cache"
	"chat/pkg/common/db/database"
	admindbmodel "chat/pkg/common/db/model/admin"
	"chat/pkg/common/imapi"
	"chat/pkg/common/kdisc"
	disetcd "chat/pkg/common/kdisc/etcd"
	"chat/pkg/common/qrlogin"
	"chat/pkg/common/rtc"
	adminclient "chat/pkg/protocol/admin"
	chatclient "chat/pkg/protocol/chat"
	"chat/pkg/common/db/mongoutil"
	"chat/pkg/common/db/redisutil"
	"chat/pkg/common/discovery/etcd"
	"chat/pkg/common/errs"
	"chat/pkg/common/log"
	"chat/pkg/common/mw"
	"chat/pkg/common/program"
	"chat/pkg/util/datautil"
	"chat/pkg/util/runtimeenv"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	ApiConfig  config.API
	Discovery  config.Discovery
	Share      config.Share
	Redis      config.Redis
	Mongo      config.Mongo
	LiveKit    config.LiveKit
	RuntimeEnv string
	ConfigPath string
}

func Start(ctx context.Context, index int, cfg *Config) error {
	cfg.RuntimeEnv = runtimeenv.RuntimeEnvironment()

	if len(cfg.Share.ChatAdmin) == 0 {
		return errs.New("share chat admin not configured")
	}
	apiPort, err := datautil.GetElemByIndex(cfg.ApiConfig.Api.Ports, index)
	if err != nil {
		return err
	}
	client, err := kdisc.NewDiscoveryRegister(&cfg.Discovery, cfg.RuntimeEnv, nil)
	if err != nil {
		return err
	}

	chatConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.Chat, grpc.WithTransportCredentials(insecure.NewCredentials()), mw.GrpcClient())
	if err != nil {
		return err
	}
	adminConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.Admin, grpc.WithTransportCredentials(insecure.NewCredentials()), mw.GrpcClient())
	if err != nil {
		return err
	}
	chatClient := chatclient.NewChatClient(chatConn)
	adminClient := adminclient.NewAdminClient(adminConn)
	im := imapi.New(cfg.Share.Server.ApiURL, cfg.Share.Server.Secret, cfg.Share.Server.AdminUserID)
	base := util.Api{
		ImUserID:        cfg.Share.Server.AdminUserID,
		ProxyHeader:     cfg.Share.ProxyHeader,
		ChatAdminUserID: cfg.Share.ChatAdmin[0],
	}

	// Initialize MeetingManager / ConferenceManager if LiveKit is configured (requires Redis)
	var meetingMgr *rtc.MeetingManager
	var conferenceMgr *rtc.ConferenceManager
	var sharedRdb redis.UniversalClient
	if cfg.LiveKit.Key != "" && cfg.LiveKit.Secret != "" {
		rdb, rdbErr := redisutil.NewRedisClient(ctx, cfg.Redis.Build())
		if rdbErr != nil {
			return errs.WrapMsg(rdbErr, "redis required for LiveKit meeting manager")
		}
		sharedRdb = rdb
		meetingMgr = rtc.NewMeetingManager(
			rdb,
			cfg.LiveKit.Key,
			cfg.LiveKit.Secret,
			cfg.LiveKit.URL,
			im,
			cfg.Share.Server.AdminUserID,
		)
		var confErr error
		conferenceMgr, confErr = rtc.NewConferenceManager(
			rdb,
			cfg.LiveKit.Key,
			cfg.LiveKit.Secret,
			cfg.LiveKit.URL,
			im,
			cfg.Share.Server.AdminUserID,
			cfg.ApiConfig.Conference.AutoStageLimit,
		)
		if confErr != nil {
			log.ZError(ctx, "ConferenceManager init failed", confErr)
		}
	}

	var qrLoginMgr *qrlogin.Manager
	if sharedRdb == nil {
		rdb, rdbErr := redisutil.NewRedisClient(ctx, cfg.Redis.Build())
		if rdbErr == nil {
			sharedRdb = rdb
		} else {
			log.ZError(ctx, "redis unavailable, qr login disabled", rdbErr)
		}
	}
	if sharedRdb != nil {
		qrLoginMgr = qrlogin.NewManager(sharedRdb, 120*time.Second)
	} else {
		log.ZWarn(ctx, "qr login requires redis but no redis connection is available", nil)
	}

	adminApi := New(chatClient, adminClient, im, &base, meetingMgr, conferenceMgr, nil, qrLoginMgr)
	// 初始化 MongoDB（红包数据库）
	mgoCli, err := mongoutil.NewMongoDB(ctx, cfg.Mongo.Build())
	if err == nil {
		rpDB, dbErr := database.NewRedPacketDatabase(mgoCli)
		if dbErr == nil {
			var rpCacheSvc *rpCache.RedPacketCache
			if sharedRdb != nil {
				rpCacheSvc = rpCache.NewRedPacketCache(sharedRdb)
			}
			rpApi := &RedPacketApi{db: rpDB, cache: rpCacheSvc, im: im, imUserID: cfg.Share.Server.AdminUserID}
			adminApi.RedPacket = rpApi
			// 启动过期红包退款后台 goroutine（每5分钟扫描一次）
			go startExpireRefundLoop(ctx, rpDB, rpCacheSvc, im, cfg.Share.Server.AdminUserID)
		}
		// 初始化支付密码 API（使用 ChatDatabase）
		chatDB, chatDBErr := database.NewChatDatabase(mgoCli)
		if chatDBErr == nil {
			adminApi.PaymentPassword = NewPaymentPasswordApi(chatDB)
			adminApi.chatDatabase = chatDB
			if conferenceMgr != nil {
				imCaller := im
				conferenceMgr.SetLifecycleRecipientLister(func(lctx context.Context) []string {
					return rtc.PickLifecycleRecipients(lctx,
						func(ctx context.Context) ([]string, error) {
							if imCaller == nil {
								return nil, fmt.Errorf("im caller unavailable")
							}
							return imCaller.GetAllOnlineUserIDs(ctx)
						},
						func(ctx context.Context) []string {
							return listChatUserIDsForLifecycle(ctx, chatDB)
						},
					)
				})
			}
		}
		// 初始化举报集合
		if reportColl, reportErr := admindbmodel.NewReport(mgoCli.GetDB()); reportErr == nil {
			adminApi.Report = NewReportApi(reportColl)
		}
		if asrColl, asrErr := admindbmodel.NewAsrModel(mgoCli.GetDB()); asrErr == nil {
			adminApi.AsrModel = NewAsrModelApi(asrColl)
		}
		// 预约会议
		if scheduleDB, scheduleErr := database.NewScheduleMeetingDatabase(mgoCli); scheduleErr == nil {
			rdbForSchedule := sharedRdb
			if rdbForSchedule == nil {
				if rdb, rdbErr := redisutil.NewRedisClient(ctx, cfg.Redis.Build()); rdbErr == nil {
					rdbForSchedule = rdb
					sharedRdb = rdb
				}
			}
			if rdbForSchedule != nil {
				sched := rtc.NewScheduleService(scheduleDB, conferenceMgr, im, cfg.Share.Server.AdminUserID, rdbForSchedule)
				adminApi.ScheduleService = sched
				sched.StartWorkers(ctx)
			} else {
				log.ZWarn(ctx, "schedule meeting requires redis but redis unavailable", nil)
			}
		} else {
			log.ZWarn(ctx, "schedule meeting database init failed", scheduleErr)
		}
	}
	mwApi := chatmw.New(adminClient)
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery(), mw.CorsHandler())
	// External webhooks do not send operationID; register before GinParseOperationID.
	engine.POST("/meeting/livekit_webhook", adminApi.LiveKitWebhook)
	engine.POST("/conference/livekit_webhook", adminApi.ConferenceLiveKitWebhook)
	engine.POST("/callback/server", adminApi.ServerCallback)
	engine.Use(mw.GinParseOperationID())
	SetChatRoute(engine, adminApi, mwApi)

	var (
		netDone = make(chan struct{}, 1)
		netErr  error
	)
	server := http.Server{Addr: fmt.Sprintf(":%d", apiPort), Handler: engine, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		err = server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			netErr = errs.WrapMsg(err, fmt.Sprintf("api start err: %s", server.Addr))
			netDone <- struct{}{}
		}
	}()
	if cfg.Discovery.Enable == kdisc.ETCDCONST {
		cm := disetcd.NewConfigManager(client.(*etcd.SvcDiscoveryRegistryImpl).GetClient(),
			[]string{
				config.ChatAPIChatCfgFileName,
				config.DiscoveryConfigFileName,
				config.ShareFileName,
				config.LiveKitConfigFileName,
				config.LogConfigFileName,
			},
		)
		cm.Watch(ctx)
	}
	shutdown := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := server.Shutdown(ctx)
		if err != nil {
			return errs.WrapMsg(err, "shutdown err")
		}
		return nil
	}
	disetcd.RegisterShutDown(shutdown)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	select {
	case <-sigs:
		program.SIGTERMExit()
		if err := shutdown(); err != nil {
			return err
		}
	case <-netDone:
		close(netDone)
		return netErr
	}
	return nil
}

func SetChatRoute(router gin.IRouter, chat *Api, mw *chatmw.MW) {
	account := router.Group("/account")
	account.POST("/code/send", chat.SendVerifyCode)                      // Send verification code
	account.POST("/code/verify", chat.VerifyCode)                        // Verify the verification code
	account.POST("/register", mw.CheckAdminOrNil, chat.RegisterUser)     // Register
	account.POST("/login", chat.Login)                                   // Login
	account.POST("/password/reset", chat.ResetPassword)                  // Forgot password
	account.POST("/password/change", mw.CheckToken, chat.ChangePassword) // Change password
	account.POST("/delete", mw.CheckToken, chat.DeleteMyAccount)         // Delete (cancel) current user's account
	account.POST("/qr/create", chat.QrLoginCreate)                       // Desktop QR login: create session
	account.GET("/qr/status", chat.QrLoginStatus)                        // Desktop QR login: poll status (legacy GET)
	account.POST("/qr/status", chat.QrLoginStatus)                       // Desktop QR login: poll status
	account.POST("/qr/scan", mw.CheckToken, chat.QrLoginScan)            // Mobile QR login: mark scanned
	account.POST("/qr/confirm", mw.CheckToken, chat.QrLoginConfirm)      // Mobile QR login: confirm
	account.POST("/qr/cancel", chat.QrLoginCancel)                       // Cancel QR login session

	user := router.Group("/user", mw.CheckToken)
	user.POST("/update", chat.UpdateUserInfo)                 // Edit personal information
	user.POST("/find/public", chat.FindUserPublicInfo)        // Get user's public information
	user.POST("/find/full", chat.FindUserFullInfo)            // Get all information of the user
	user.POST("/search/full", chat.SearchUserFullInfo)        // Search user's public information
	user.POST("/search/public", chat.SearchUserPublicInfo) // Search all information of the user

	// Meeting / Call management
	meeting := router.Group("/meeting", mw.CheckToken)
	meeting.POST("/create", chat.CreateMeeting)
	meeting.POST("/join", chat.JoinMeeting)
	meeting.POST("/leave", chat.LeaveMeeting)
	meeting.POST("/end", chat.EndMeeting)
	meeting.POST("/get", chat.GetMeeting)
	meeting.POST("/get_active", chat.GetUserActiveMeeting)

	// Independent large conference mode
	conference := router.Group("/conference", mw.CheckToken)
	conference.POST("/create", chat.CreateConference)
	conference.POST("/join", chat.JoinConference)
	conference.POST("/leave", chat.LeaveConference)
	conference.POST("/end", chat.EndConference)
	conference.POST("/share", chat.ShareConference)
	conference.POST("/register_announce", chat.RegisterConferenceAnnounce)
	conference.POST("/get", chat.GetConference)
	conference.POST("/list_active", chat.ListActiveConferences)
	conference.POST("/participants", chat.ListConferenceParticipants)
	conference.POST("/invite", chat.InviteConference)
	conference.POST("/transfer_admin", chat.TransferConferenceAdmin)
	conference.POST("/set_cohost", chat.SetConferenceCohost)
	conference.POST("/kick", chat.KickConference)
	conference.POST("/remove", chat.RemoveConference)
	conference.POST("/media", chat.SetConferenceMedia)
	conference.POST("/media_all", chat.SetConferenceMediaAll)
	conference.POST("/speak/apply", chat.ApplyConferenceSpeak)
	conference.POST("/speak/approve", chat.ApproveConferenceSpeak)
	conference.POST("/speak/leave", chat.LeaveConferenceSpeak)
	conference.POST("/link_mic", chat.SetConferenceLinkMic)
	conference.POST("/camera/toggle", chat.ToggleConferenceCamera)
	conference.POST("/lock", chat.LockConference)
	conference.POST("/chat_mute", chat.ChatMuteConference)
	conference.POST("/member_chat_mute", chat.MemberChatMuteConference)
	conference.POST("/revoke_chat", chat.RevokeConferenceChat)
	conference.POST("/pin", chat.PinConference)
	conference.POST("/egress/start", chat.StartConferenceEgress)
	conference.POST("/egress/stop", chat.StopConferenceEgress)
	conference.POST("/ingress/create", chat.CreateConferenceIngress)
	conference.POST("/ingress/delete", chat.DeleteConferenceIngress)

	// 预约会议
	if chat.ScheduleService != nil {
		schedule := router.Group("/conference/schedule", mw.CheckToken)
		schedule.POST("/create", chat.CreateScheduleMeeting)
		schedule.POST("/update", chat.UpdateScheduleMeeting)
		schedule.POST("/cancel", chat.CancelScheduleMeeting)
		schedule.POST("/list", chat.ListScheduleMeeting)
		schedule.POST("/get", chat.GetScheduleMeeting)
		schedule.POST("/join", chat.JoinScheduleMeeting)
		schedule.POST("/start", chat.StartScheduleMeeting)
		schedule.POST("/end", chat.EndScheduleMeeting)
		schedule.POST("/waiting/list", chat.ListConferenceWaiting)
		schedule.POST("/waiting/admit", chat.AdmitConferenceWaiting)
		schedule.POST("/waiting/reject", chat.RejectConferenceWaiting)
		schedule.POST("/waiting/admit_all", chat.AdmitAllConferenceWaiting)
		schedule.POST("/waiting/leave", chat.LeaveConferenceWaiting)

		waiting := router.Group("/conference/waiting", mw.CheckToken)
		waiting.POST("/list", chat.ListConferenceWaiting)
		waiting.POST("/admit", chat.AdmitConferenceWaiting)
		waiting.POST("/reject", chat.RejectConferenceWaiting)
		waiting.POST("/admit_all", chat.AdmitAllConferenceWaiting)
		waiting.POST("/leave", chat.LeaveConferenceWaiting)
	}

	router.POST("/friend/search", mw.CheckToken, chat.SearchFriend)

	// 支付密码
	if chat.PaymentPassword != nil {
		payPwd := router.Group("/user/payment_password", mw.CheckToken)
		payPwd.POST("/set", chat.PaymentPassword.SetPaymentPassword)
		payPwd.POST("/change", chat.PaymentPassword.ChangePaymentPassword)
		payPwd.POST("/verify", chat.PaymentPassword.VerifyPaymentPassword)
		payPwd.POST("/check", chat.PaymentPassword.CheckPaymentPasswordSet)
		payPwd.POST("/reset", chat.PaymentPassword.ResetPaymentPassword)
	}

	router.Group("/applet").POST("/find", mw.CheckToken, chat.FindApplet) // Applet list

	router.Group("/client_config").POST("/get", chat.GetClientConfig) // Get client initialization configuration

	if chat.AsrModel != nil {
		router.Group("/asr_model").POST("/list", chat.AsrModel.List)
	}

	moment := router.Group("/moment", mw.CheckToken)
	moment.POST("/create", chat.CreateMoment)
	moment.POST("/delete", chat.DeleteMoment)
	moment.POST("/list", chat.ListMoments)
	moment.POST("/comment", chat.CommentMoment)
	moment.POST("/delete_comment", chat.DeleteComment)
	moment.POST("/like", chat.LikeMoment)
	moment.POST("/unlike", chat.UnlikeMoment)

	friendTag := router.Group("/friend_tag", mw.CheckToken)
	friendTag.POST("/create", chat.CreateFriendTag)
	friendTag.POST("/rename", chat.RenameFriendTag)
	friendTag.POST("/delete", chat.DeleteFriendTag)
	friendTag.POST("/add_members", chat.AddFriendTagMembers)
	friendTag.POST("/remove_members", chat.RemoveFriendTagMembers)
	friendTag.POST("/list", chat.ListFriendTags)
	friendTag.POST("/list_members", chat.ListFriendTagMembers)

	favorite := router.Group("/favorite", mw.CheckToken)
	favorite.POST("/add", chat.AddFavorite)
	favorite.POST("/remove", chat.RemoveFavorite)
	favorite.POST("/list", chat.ListFavorites)
	favorite.POST("/update", chat.UpdateFavorite)

	applicationGroup := router.Group("application")
	applicationGroup.POST("/latest_version", chat.LatestApplicationVersion)

	// 红包 & 积分（用户端与管理员端，防止 MongoDB 未就绪时 nil 指针 panic）
	if chat.RedPacket != nil {
		redPacket := router.Group("/red_packet", mw.CheckToken)
		redPacket.POST("/send", chat.RedPacket.SendRedPacket)
		redPacket.POST("/grab", chat.RedPacket.GrabRedPacket)
		redPacket.POST("/detail", chat.RedPacket.GetRedPacketDetail)

		points := router.Group("/points", mw.CheckToken)
		points.POST("/balance", chat.RedPacket.GetPointsBalance)
		points.POST("/transactions", chat.RedPacket.GetPointsTransactions)

		adminPoints := router.Group("/admin/points", mw.CheckAdmin)
		adminPoints.POST("/adjust", chat.RedPacket.AdminAdjustPoints)
		adminPoints.POST("/list", chat.RedPacket.AdminListPoints)
		adminPoints.POST("/transactions", chat.RedPacket.AdminListTransactions)

		adminRedPacket := router.Group("/admin/red_packets", mw.CheckAdmin)
		adminRedPacket.POST("/list", chat.RedPacket.AdminListRedPackets)
		adminRedPacket.POST("/grabs", chat.RedPacket.AdminListGrabs)
	}

	// 会议管理（管理员端：全部进行中列表 + 调整显示人数 + 强制结束）——走 chat-api 的 ConferenceManager，
	// 与进/退会共用同一条 conference_sync 推送路径。路径按作用命名，避开网关 /admin/ 前缀。
	if chat.ConferenceManager != nil {
		conferenceAdmin := NewConferenceAdminApi(chat.ConferenceManager)
		conferenceOps := router.Group("/conference", mw.CheckAdmin)
		conferenceOps.POST("/list_all", conferenceAdmin.AdminListActiveConferences)
		conferenceOps.POST("/adjust_display_count", conferenceAdmin.AdminAdjustConferenceDisplayCount)
		conferenceOps.POST("/force_end", conferenceAdmin.AdminForceEndConference)
	}

	// 举报（用户侧）
	if chat.Report != nil {
		router.Group("/report").POST("/create", mw.CheckToken, chat.Report.CreateReport)
	}
}

func listChatUserIDsForLifecycle(ctx context.Context, db database.ChatDatabaseInterface) []string {
	if db == nil {
		return nil
	}
	const pageSize int32 = 200
	const maxPages int32 = 25
	var ids []string
	for page := int32(1); page <= maxPages; page++ {
		total, attrs, err := db.Search(ctx, 0, "", 0, newPage(page, pageSize))
		if err != nil {
			log.ZWarn(ctx, "conference lifecycle user list failed", err, "page", page)
			break
		}
		for _, a := range attrs {
			if a != nil && a.UserID != "" {
				ids = append(ids, a.UserID)
			}
		}
		if len(attrs) == 0 || int64(page*pageSize) >= total {
			break
		}
	}
	return ids
}
