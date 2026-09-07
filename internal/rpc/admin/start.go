package admin

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"math/rand"
	"time"

	"chat/pkg/common/config"
	"chat/pkg/common/constant"
	"chat/pkg/common/db/database"
	"chat/pkg/common/db/dbutil"
	"chat/pkg/common/db/table/admin"
	"chat/pkg/common/tokenverify"
	adminpb "chat/pkg/protocol/admin"
	"chat/pkg/protocol/chat"
	chatClient "chat/pkg/rpclient/chat"
	"gitee.com/matkurban/openimsdk-tools/db/mongoutil"
	"gitee.com/matkurban/openimsdk-tools/db/redisutil"
	"gitee.com/matkurban/openimsdk-tools/discovery"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"gitee.com/matkurban/openimsdk-tools/mw"
	"gitee.com/matkurban/openimsdk-tools/utils/runtimeenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	RpcConfig     config.Admin
	RedisConfig   config.Redis
	MongodbConfig config.Mongo
	Discovery     config.Discovery
	Share         config.Share

	RuntimeEnv string
}

func Start(ctx context.Context, config *Config, client discovery.SvcDiscoveryRegistry, server *grpc.Server) error {
	config.RuntimeEnv = runtimeenv.RuntimeEnvironment()

	if len(config.Share.ChatAdmin) == 0 {
		return errs.New("share chat admin not configured")
	}
	rand.Seed(time.Now().UnixNano())
	rdb, err := redisutil.NewRedisClient(ctx, config.RedisConfig.Build())
	if err != nil {
		return err
	}
	mgocli, err := mongoutil.NewMongoDB(ctx, config.MongodbConfig.Build())
	if err != nil {
		return err
	}
	var srv adminServer
	srv.Database, err = database.NewAdminDatabase(mgocli, rdb)
	if err != nil {
		return err
	}
	conn, err := client.GetConn(ctx, config.Discovery.RpcService.Chat, grpc.WithTransportCredentials(insecure.NewCredentials()), mw.GrpcClient())
	if err != nil {
		return err
	}
	srv.Chat = chatClient.NewChatClient(chat.NewChatClient(conn))
	srv.Token = &tokenverify.Token{
		Expires: time.Duration(config.RpcConfig.TokenPolicy.Expire) * time.Hour * 24,
		Secret:  config.RpcConfig.Secret,
	}
	if err := srv.initAdmin(ctx, config.Share.ChatAdmin, config.Share.Server.AdminUserID); err != nil {
		return err
	}
	adminpb.RegisterAdminServer(server, &srv)
	return nil
}

type adminServer struct {
	adminpb.UnimplementedAdminServer
	Database database.AdminDatabaseInterface
	Chat     *chatClient.ChatClient
	Token    *tokenverify.Token
}

func (o *adminServer) initAdmin(ctx context.Context, admins []string, imUserID string) error {
	for _, account := range admins {
		if _, err := o.Database.GetAdmin(ctx, account); err == nil {
			continue
		} else if !dbutil.IsDBNotFound(err) {
			return err
		}
		sum := md5.Sum([]byte(account))
		a := admin.Admin{
			Account:    account,
			UserID:     imUserID,
			Password:   hex.EncodeToString(sum[:]),
			Level:      constant.DefaultAdminLevel,
			CreateTime: time.Now(),
		}
		if err := o.Database.AddAdminAccount(ctx, []*admin.Admin{&a}); err != nil {
			return err
		}
	}
	return nil
}
