# chat

业务服务（账号、管理后台、机器人、会议、积分红包/转账），Go module `chat`。进程与配置文件统一使用 `chat-*` 命名，构建/运行统一使用 [Mage](https://magefile.org/)，与 [xue-hua-server](../xue-hua-server) 一致。

## 快速开始

本项目在 Linux/Windows/Mac 平台以及 ARM 和 AMD 架构下均可正常使用。

### 初始化

第一次编译前，Linux/Mac 平台下执行 `sh bootstrap.sh`，Windows 执行 `bootstrap.bat`。

### 编译 / 启动 / 检测 / 停止

```bash
mage                                        # 编译全部服务与工具
mage buildOnly "chat-api-chat check-component"   # 只编译指定目标
mage start
mage check
mage stop
```

### 启动顺序

1. 在 xue-hua-server 目录 `docker compose up -d` 起好基础设施（MongoDB、Redis、etcd、Kafka、MinIO、LiveKit），再 `mage start` 启动 IM server。
2. 本项目 `mage` 编译，`mage start` 启动。

`ps` 中的进程名：`chat-api-chat`、`chat-rpc-chat`、`chat-api-admin`、`chat-rpc-admin`、`chat-api-bot`、`chat-rpc-bot`，由 `start-config.yml` 控制启动哪些进程与实例数。`chat-api-chat` 默认监听 `10008`。

### 配置

- `config/mongodb.yml`：数据库 `chat_v3`；`config/redis.yml`；`config/discovery.yml`：etcd 根目录 `chat`。
- `config/share.yml` 的 `server.apiURL` / `server.secret` / `server.adminUserID` 指向 xue-hua-server（默认 `http://127.0.0.1:10002`，与其 `config/share.yml` 一致）。
- `config/livekit.yml`：LiveKit 由 xue-hua-server 的 docker-compose 部署，key/secret 必须与其 `.env` 中的 `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` 一致。
- 环境变量覆盖前缀 `CHATENV_*`，例如 `CHATENV_REDIS_PASSWORD`。

积分红包与积分转账使用 MongoDB 集合 `red_packets`、`red_packet_grabs`、`points*`。历史上的 Web3 红包/转账已移除；旧库中如有 `chat_web3_packets`、`chat_web3_packet_claims`、`conference_red_packets`、`conference_red_packet_claims`、`user_payfi_principals` 集合，可在备份后手动 drop。

## 交叉编译 Linux amd64 并打包

```bash
CGO_ENABLED=0 PLATFORMS=linux_amd64 mage
mage pack        # 产物 _output/chat-linux-amd64.zip
```

产物内容（解压后保持同样目录结构）：

| 路径 | 用途 |
| --- | --- |
| `_output/bin/platforms/linux/amd64/` | 6 个 `chat-*` 进程 |
| `_output/bin/tools/linux/amd64/` | `check-component`、`attribute-to-credential` |
| `config/` | 全部 yml |
| `start-config.yml` | 启哪些进程 |
| `mage` | 已编译的 Linux/amd64 mage 启动器，服务器无需安装 Go |
| `magefile*.go`、`mage_pack.go`、`go.mod`、`go.sum`、`version/` | mage 运行时文件 |

上传并解压后在服务器执行：

```bash
cd /opt/chat
./mage start
nohup ./mage start >> _output/logs/chat.log 2>&1 &   # 长期运行
./mage check
./mage stop
```

没有 linux/amd64 二进制时 `mage pack` 会失败，提示先交叉编译。

## 容器

```bash
docker build -t chat:local .
docker run --rm -v "$PWD/config:/chat/config:ro" chat:local /chat/bin/chat-api-chat -c /chat/config
```

各服务镜像模板见 `build/images/`，Kubernetes 清单见 `deployments/`。

## 致谢

源自 [OpenIM chat](https://github.com/openimsdk/chat)，保留原有版权声明，Apache-2.0 许可。
