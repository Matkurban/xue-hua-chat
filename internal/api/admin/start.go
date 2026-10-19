package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	chatapi "chat/internal/api/chat"
	chatmw "chat/internal/api/mw"
	"chat/internal/api/util"
	"chat/pkg/common/appversion"
	"chat/pkg/common/captcha"
	"chat/pkg/common/config"
	rpCache "chat/pkg/common/db/cache"
	"chat/pkg/common/db/database"
	admindbmodel "chat/pkg/common/db/model/admin"
	"chat/pkg/common/imapi"
	"chat/pkg/common/kdisc"
	disetcd "chat/pkg/common/kdisc/etcd"
	"chat/pkg/common/storage"
	adminclient "chat/pkg/protocol/admin"
	chatclient "chat/pkg/protocol/chat"
	"gitee.com/matkurban/openimsdk-tools/db/mongoutil"
	"gitee.com/matkurban/openimsdk-tools/db/redisutil"
	"gitee.com/matkurban/openimsdk-tools/discovery"
	"gitee.com/matkurban/openimsdk-tools/discovery/etcd"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"gitee.com/matkurban/openimsdk-tools/log"
	"gitee.com/matkurban/openimsdk-tools/mw"
	"gitee.com/matkurban/openimsdk-tools/system/program"
	"gitee.com/matkurban/openimsdk-tools/utils/datautil"
	"gitee.com/matkurban/openimsdk-tools/utils/runtimeenv"
	"github.com/gin-gonic/gin"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	*config.AllConfig

	RuntimeEnv string
	ConfigPath string
}

func Start(ctx context.Context, index int, config *Config) error {
	config.RuntimeEnv = runtimeenv.RuntimeEnvironment()

	if len(config.Share.ChatAdmin) == 0 {
		return errs.New("share chat admin not configured")
	}
	apiPort, err := datautil.GetElemByIndex(config.AdminAPI.Api.Ports, index)
	if err != nil {
		return err
	}
	client, err := kdisc.NewDiscoveryRegister(&config.Discovery, config.RuntimeEnv, nil)
	if err != nil {
		return err
	}

	chatConn, err := client.GetConn(ctx, config.Discovery.RpcService.Chat, grpc.WithTransportCredentials(insecure.NewCredentials()), mw.GrpcClient())
	if err != nil {
		return err
	}
	adminConn, err := client.GetConn(ctx, config.Discovery.RpcService.Admin, grpc.WithTransportCredentials(insecure.NewCredentials()), mw.GrpcClient())
	if err != nil {
		return err
	}
	chatClient := chatclient.NewChatClient(chatConn)
	adminClient := adminclient.NewAdminClient(adminConn)
	im := imapi.New(config.Share.Server.ApiURL, config.Share.Server.Secret, config.Share.Server.AdminUserID)
	base := util.Api{
		ImUserID:        config.Share.Server.AdminUserID,
		ProxyHeader:     config.Share.ProxyHeader,
		ChatAdminUserID: config.Share.ChatAdmin[0],
	}
	adminApi := New(chatClient, adminClient, im, &base)
	adminApi.Prober = appversion.NewProber()
	pkgDir := config.AdminAPI.AppPackage.Dir
	if pkgDir == "" {
		pkgDir = "app_packages"
	}
	if !filepath.IsAbs(pkgDir) && config.ConfigPath != "" {
		if abs, absErr := filepath.Abs(filepath.Join(config.ConfigPath, "..", pkgDir)); absErr == nil {
			pkgDir = abs
		}
	}
	adminApi.PackageBaseURL = strings.TrimRight(config.AdminAPI.AppPackage.BaseURL, "/")
	if ps, psErr := storage.NewPackageStorage(pkgDir, "/application/files", storage.DefaultMaxPackageSize); psErr != nil {
		log.ZWarn(ctx, "app package storage init failed, upload disabled", psErr)
	} else {
		adminApi.Packages = ps
		go func() { // backfill the instant-upload index for files stored before it existed; never blocks startup
			start := time.Now()
			r, bErr := ps.Backfill()
			log.ZInfo(ctx, "app package index backfill done", "dir", ps.BaseDir, "scanned", r.Scanned, "indexed", r.Indexed,
				"skipped", r.Skipped, "failed", r.Failed, "err", bErr, "cost", time.Since(start).String())
		}()
	}
	mwApi := chatmw.New(adminClient)

	// 初始化 RedPacketApi（用于管理员积分/红包查询接口）
	var redPacket *chatapi.RedPacketApi
	var governanceApi *GovernanceApi
	mgoCli, mongoErr := mongoutil.NewMongoDB(ctx, config.Mongo.Build())
	rdb, rdbErr := redisutil.NewRedisClient(ctx, config.Redis.Build())
	if mongoErr == nil {
		rpDB, dbErr := database.NewRedPacketDatabase(mgoCli)
		if dbErr == nil {
			var rpCacheSvc *rpCache.RedPacketCache
			if rdbErr == nil {
				rpCacheSvc = rpCache.NewRedPacketCache(rdb)
			}
			redPacket = chatapi.NewRedPacketApi(rpDB, rpCacheSvc, im, config.Share.Server.AdminUserID)
		}
		// 初始化 Governance（举报 + 申诉）
		reportColl, repErr := admindbmodel.NewReport(mgoCli.GetDB())
		appealColl, appErr := admindbmodel.NewAppeal(mgoCli.GetDB())
		if rdbErr == nil && repErr == nil && appErr == nil {
			captchaSvc := captcha.New(rdb, 5*time.Minute)
			tokenMgr := chatmw.NewAppealTokenManager(rdb, 30*time.Minute)
			st, stErr := storage.NewAppealStorage("./appeal_evidence", "/appeal/files", storage.DefaultMaxFileSize)
			if stErr == nil {
				governanceApi = NewGovernanceApi(reportColl, appealColl, captchaSvc, tokenMgr, st, adminClient)
				_ = st
			}
		}
		if asrColl, asrErr := admindbmodel.NewAsrModel(mgoCli.GetDB()); asrErr == nil {
			adminApi.AsrModel = NewAsrModelApi(asrColl)
		}
		if notifColl, notifErr := admindbmodel.NewSystemNotification(mgoCli.GetDB()); notifErr != nil {
			log.ZWarn(ctx, "system notification store init failed", notifErr)
		} else if targetColl, tErr := admindbmodel.NewSystemNotificationTarget(mgoCli.GetDB()); tErr != nil {
			log.ZWarn(ctx, "system notification target store init failed", tErr)
		} else {
			adminApi.SystemNotification = NewSystemNotificationApi(notifColl, targetColl, im, config.Share.Server.AdminUserID)
		}
	}

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery(), mw.CorsHandler(), mw.GinParseOperationID())
	SetAdminRoute(engine, adminApi, mwApi, config, client, redPacket, governanceApi)

	if config.Discovery.Enable == kdisc.ETCDCONST {
		cm := disetcd.NewConfigManager(client.(*etcd.SvcDiscoveryRegistryImpl).GetClient(), config.GetConfigNames())
		cm.Watch(ctx)
	}
	var (
		netDone = make(chan struct{}, 1)
		netErr  error
	)
	server := http.Server{Addr: fmt.Sprintf(":%d", apiPort), Handler: engine}
	go func() {
		err = server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			netErr = errs.WrapMsg(err, fmt.Sprintf("api start err: %s", server.Addr))
			netDone <- struct{}{}
		}
	}()
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

func SetAdminRoute(router gin.IRouter, admin *Api, mw *chatmw.MW, cfg *Config, client discovery.SvcDiscoveryRegistry, redPacket *chatapi.RedPacketApi, governance *GovernanceApi) {

	adminRouterGroup := router.Group("/account")
	adminRouterGroup.POST("/login", admin.AdminLogin)                                   // Login
	adminRouterGroup.POST("/update", mw.CheckAdmin, admin.AdminUpdateInfo)              // Modify information
	adminRouterGroup.POST("/info", mw.CheckAdmin, admin.AdminInfo)                      // Get information
	adminRouterGroup.POST("/change_password", mw.CheckAdmin, admin.ChangeAdminPassword) // Change admin account's password
	adminRouterGroup.POST("/add_admin", mw.CheckAdmin, admin.AddAdminAccount)           // Add admin account
	adminRouterGroup.POST("/add_user", mw.CheckAdmin, admin.AddUserAccount)             // Add user account
	adminRouterGroup.POST("/del_admin", mw.CheckAdmin, admin.DelAdminAccount)           // Delete admin
	adminRouterGroup.POST("/search", mw.CheckAdmin, admin.SearchAdminAccount)           // Get admin list
	//account.POST("/add_notification_account")

	importGroup := router.Group("/user/import")
	importGroup.POST("/json", mw.CheckAdmin, admin.ImportUserByJson)
	importGroup.POST("/xlsx", mw.CheckAdmin, admin.ImportUserByXlsx)
	importGroup.GET("/xlsx", admin.BatchImportTemplate)

	allowRegisterGroup := router.Group("/user/allow_register", mw.CheckAdmin)
	allowRegisterGroup.POST("/get", admin.GetAllowRegister)
	allowRegisterGroup.POST("/set", admin.SetAllowRegister)

	defaultRouter := router.Group("/default", mw.CheckAdmin)
	defaultUserRouter := defaultRouter.Group("/user")
	defaultUserRouter.POST("/add", admin.AddDefaultFriend)       // Add default friend at registration
	defaultUserRouter.POST("/del", admin.DelDefaultFriend)       // Delete default friend at registration
	defaultUserRouter.POST("/find", admin.FindDefaultFriend)     // Default friend list
	defaultUserRouter.POST("/search", admin.SearchDefaultFriend) // Search default friend list at registration
	defaultGroupRouter := defaultRouter.Group("/group")
	defaultGroupRouter.POST("/add", admin.AddDefaultGroup)       // Add default group at registration
	defaultGroupRouter.POST("/del", admin.DelDefaultGroup)       // Delete default group at registration
	defaultGroupRouter.POST("/find", admin.FindDefaultGroup)     // Get default group list at registration
	defaultGroupRouter.POST("/search", admin.SearchDefaultGroup) // Search default group list at registration

	invitationCodeRouter := router.Group("/invitation_code", mw.CheckAdmin)
	invitationCodeRouter.POST("/add", admin.AddInvitationCode)       // Add invitation code
	invitationCodeRouter.POST("/gen", admin.GenInvitationCode)       // Generate invitation code
	invitationCodeRouter.POST("/del", admin.DelInvitationCode)       // Delete invitation code
	invitationCodeRouter.POST("/search", admin.SearchInvitationCode) // Search invitation code

	forbiddenRouter := router.Group("/forbidden", mw.CheckAdmin)
	ipForbiddenRouter := forbiddenRouter.Group("/ip")
	ipForbiddenRouter.POST("/add", admin.AddIPForbidden)       // Add forbidden IP for registration/login
	ipForbiddenRouter.POST("/del", admin.DelIPForbidden)       // Delete forbidden IP for registration/login
	ipForbiddenRouter.POST("/search", admin.SearchIPForbidden) // Search forbidden IPs for registration/login
	userForbiddenRouter := forbiddenRouter.Group("/user")
	userForbiddenRouter.POST("/add", admin.AddUserIPLimitLogin)       // Add limit for user login on specific IP
	userForbiddenRouter.POST("/del", admin.DelUserIPLimitLogin)       // Delete user limit on specific IP for login
	userForbiddenRouter.POST("/search", admin.SearchUserIPLimitLogin) // Search limit for user login on specific IP

	appletRouterGroup := router.Group("/applet", mw.CheckAdmin)
	appletRouterGroup.POST("/add", admin.AddApplet)       // Add applet
	appletRouterGroup.POST("/del", admin.DelApplet)       // Delete applet
	appletRouterGroup.POST("/update", admin.UpdateApplet) // Modify applet
	appletRouterGroup.POST("/search", admin.SearchApplet) // Search applet

	blockRouter := router.Group("/block", mw.CheckAdmin)
	blockRouter.POST("/add", admin.BlockUser)          // Block user
	blockRouter.POST("/del", admin.UnblockUser)        // Unblock user
	blockRouter.POST("/search", admin.SearchBlockUser) // Search blocked users

	userRouter := router.Group("/user", mw.CheckAdmin)
	userRouter.POST("/password/reset", admin.ResetUserPassword) // Reset user password

	initGroup := router.Group("/client_config", mw.CheckAdmin)
	initGroup.POST("/get", admin.GetClientConfig) // Get client initialization configuration
	initGroup.POST("/set", admin.SetClientConfig) // Set client initialization configuration
	initGroup.POST("/del", admin.DelClientConfig) // Delete client initialization configuration

	statistic := router.Group("/statistic", mw.CheckAdmin)
	statistic.POST("/new_user_count", admin.NewUserCount)
	statistic.POST("/login_user_count", admin.LoginUserCount)

	applicationGroup := router.Group("application")
	applicationGroup.POST("/add_version", mw.CheckAdmin, admin.AddApplicationVersion)
	applicationGroup.POST("/update_version", mw.CheckAdmin, admin.UpdateApplicationVersion)
	applicationGroup.POST("/delete_version", mw.CheckAdmin, admin.DeleteApplicationVersion)
	applicationGroup.POST("/revoke_version", mw.CheckAdmin, admin.RevokeApplicationVersion)
	applicationGroup.POST("/upload_package", mw.CheckAdmin, admin.UploadApplicationPackage)
	applicationGroup.POST("/upload", mw.CheckAdmin, admin.UploadApplicationPackage) // alias
	applicationGroup.POST("/check_package", mw.CheckAdmin, admin.CheckApplicationPackage)
	applicationGroup.POST("/probe_url", mw.CheckAdmin, admin.ProbeApplicationURL)
	if admin.Packages != nil {
		// uploaded packages are downloaded by clients without auth
		// dot-paths (.index, temp files) and directory listings are 404
		fh := gin.WrapH(admin.Packages.FileHandler())
		router.GET("/application/files/*filepath", fh)
		router.HEAD("/application/files/*filepath", fh)
	}
	applicationGroup.POST("/latest_version", admin.LatestApplicationVersion)
	applicationGroup.POST("/page_versions", mw.CheckAdmin, admin.PageApplicationVersion)

	if admin.AsrModel != nil {
		asrGroup := router.Group("/asr_model", mw.CheckAdmin)
		asrGroup.POST("/list", admin.AsrModel.List)
		asrGroup.POST("/upsert", admin.AsrModel.Upsert)
	}

	if admin.SystemNotification != nil {
		snGroup := router.Group("/system_notification", mw.CheckAdmin)
		snGroup.POST("/send", admin.SystemNotification.Send)
		snGroup.POST("/list", admin.SystemNotification.List)
		snGroup.POST("/revoke", admin.SystemNotification.Revoke)
	}

	var etcdClient *clientv3.Client
	if cfg.Discovery.Enable == kdisc.ETCDCONST {
		etcdClient = client.(*etcd.SvcDiscoveryRegistryImpl).GetClient()
	}
	cm := NewConfigManager(cfg.AllConfig, etcdClient, cfg.ConfigPath, cfg.RuntimeEnv)
	{
		configGroup := router.Group("/config", mw.CheckAdmin)
		configGroup.POST("/get_config_list", cm.GetConfigList)
		configGroup.POST("/get_config", cm.GetConfig)
		configGroup.POST("/set_config", cm.SetConfig)
		configGroup.POST("/set_configs", cm.SetConfigs)
		configGroup.POST("/reset_config", cm.ResetConfig)
		configGroup.POST("/get_enable_config_manager", cm.GetEnableConfigManager)
		configGroup.POST("/set_enable_config_manager", cm.SetEnableConfigManager)
	}
	{
		router.POST("/restart", mw.CheckAdmin, cm.Restart)
	}

	if redPacket != nil {
		adminPoints := router.Group("/admin/points", mw.CheckAdmin)
		adminPoints.POST("/adjust", redPacket.AdminAdjustPoints)
		adminPoints.POST("/list", redPacket.AdminListPoints)
		adminPoints.POST("/transactions", redPacket.AdminListTransactions)

		adminRedPacket := router.Group("/admin/red_packets", mw.CheckAdmin)
		adminRedPacket.POST("/list", redPacket.AdminListRedPackets)
		adminRedPacket.POST("/grabs", redPacket.AdminListGrabs)
	}

	{
		// 朋友圈管理：透传到 xue-hua-server /moment/admin/*
		momentAdmin := router.Group("/admin/moment/admin", mw.CheckAdmin)
		momentAdmin.POST("/list", admin.MomentAdminList)
		momentAdmin.POST("/detail", admin.MomentAdminDetail)
		momentAdmin.POST("/delete", admin.MomentAdminDelete)
		momentAdmin.POST("/comment_list", admin.MomentAdminCommentList)
		momentAdmin.POST("/delete_comment", admin.MomentAdminDeleteComment)
		momentAdmin.POST("/like_list", admin.MomentAdminLikeList)
		momentAdmin.POST("/ban", admin.MomentAdminBan)
		momentAdmin.POST("/restore", admin.MomentAdminRestore)
		momentAdmin.POST("/stats", admin.MomentAdminStats)
	}

	if governance != nil {
		// 静态证据文件服务
		router.Static("/appeal/files", "./appeal_evidence")

		// 管理端（需管理员鉴权）
		reportAdmin := router.Group("/governance/report", mw.CheckAdmin)
		reportAdmin.POST("/search", governance.SearchReport)
		reportAdmin.POST("/detail", governance.ReportDetail)
		reportAdmin.POST("/handle", governance.HandleReport)

		appealAdmin := router.Group("/governance/appeal", mw.CheckAdmin)
		appealAdmin.POST("/search", governance.SearchAppeal)
		appealAdmin.POST("/detail", governance.AppealDetail)
		appealAdmin.POST("/handle", governance.HandleAppeal)

		// 公开端（无需登录）
		pub := router.Group("/public/appeal")
		pub.POST("/captcha", governance.RequestAppealCaptcha)
		pub.POST("/create", governance.CreateAppeal)
		// 证据上传（需 appeal token）
		pub.POST("/upload", governance.AppealTokens.CheckAppealToken, governance.UploadAppealEvidence)
	}

}
