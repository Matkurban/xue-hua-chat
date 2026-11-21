package bot

import (
	"context"
	"net/http"
	"time"

	"chat/pkg/common/config"
	"chat/pkg/common/db/database"
	"chat/pkg/common/imapi"
	"chat/pkg/protocol/bot"
	"chat/pkg/common/db/mongoutil"
	"chat/pkg/common/discovery"
	"google.golang.org/grpc"
)

type Config struct {
	RpcConfig     config.Bot
	RedisConfig   config.Redis
	MongodbConfig config.Mongo
	Discovery     config.Discovery
	Share         config.Share
}

func Start(ctx context.Context, config *Config, client discovery.SvcDiscoveryRegistry, server *grpc.Server) error {
	mgocli, err := mongoutil.NewMongoDB(ctx, config.MongodbConfig.Build())
	if err != nil {
		return err
	}
	var srv botSvr

	srv.database, err = database.NewBotDatabase(mgocli)
	if err != nil {
		return err
	}
	srv.timeout = config.RpcConfig.Timeout
	srv.httpClient = &http.Client{
		Timeout: time.Duration(config.RpcConfig.Timeout) * time.Second,
	}
	im := imapi.New(config.Share.Server.ApiURL, config.Share.Server.Secret, config.Share.Server.AdminUserID)
	srv.imCaller = im
	bot.RegisterBotServer(server, &srv)
	return nil
}

type botSvr struct {
	bot.UnimplementedBotServer
	database   database.BotDatabase
	httpClient *http.Client
	timeout    int
	imCaller   imapi.CallerInterface
	//Admin           *chatClient.AdminClient
}
