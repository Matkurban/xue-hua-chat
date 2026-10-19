# 应用更新（app-update）

集合 `application`，每条记录 = 一个 `platform/arch/version/build` 的安装包。"最新版"由服务端计算，不再手工维护 `latest/hot`。

## 模型（JSON 全部 snake_case）
`id, platform(android|ios|windows|macos|linux), arch(universal|arm64|armv7|x64|x86，见下“架构”), version(x.y.z), build(int), url(可空，iOS 常空，非空须 http/https), size(bytes), sha256(64位小写hex或空), package_type(apk|exe|dmg|zip|deb|ipa|tar.gz 或空), text, min_version, force, status(published|revoked，默认 published), create_time(毫秒)`
唯一索引 `uniq_release(platform, arch, version, build)`。

## 规则（pkg/common/appversion，有单测）
- 候选 = status=published 且 arch 等于客户端 arch 或 `universal`；取 (version, build) 最大者为 `latest`；同版本同 build 时精确 arch 优先于 universal。
- `has_update` = latest 的 (version, build) > 客户端的 (version, build)（版本按数字比较，build 仅用于同版本比较）。
- `force`（仅 has_update 时）= latest.force OR 客户端 version < latest.min_version。
- 无记录 → `has_update=false, force=false, latest=null`。
- 修改记录不能改 platform/arch/version/build（请求里没有这些字段，传了也忽略）。

## 架构（arch）
规范值只有 `universal | arm64 | armv7 | x64 | x86`，与 Flutter 客户端上报一致（Android 取 supportedAbis：arm64-v8a→arm64、armeabi-v7a→armv7、x86_64→x64、x86→x86；桌面取 Dart `Abi.current()`；macOS/iOS 固定 universal）。
服务端在 add_version、update_version（老记录的别名在下一次修改时改写为规范值）、latest_version 入参、以及返回的记录上统一做归一化（`appversion.NormalizeArch`，不区分大小写、去空格）：

| 别名 | 规范值 |
|---|---|
| 空、all、any、noarch | universal |
| arm64-v8a、aarch64、armv8、arm64e | arm64 |
| armeabi-v7a、armeabi、armv7l、armv7a、armhf、arm | armv7 |
| amd64、x86_64、x86-64、x8664 | x64 |
| i386、i686、ia32、386 | x86 |

### 合法的 platform/arch 组合（`appversion.Validate`，所有写入路径都会检查）
| platform | 允许的 arch |
|---|---|
| android | universal, arm64, armv7, x64, x86 |
| windows | universal, x64, arm64, x86 |
| macos | universal, arm64, x64 |
| linux | universal, x64, arm64, armv7 |
| ios | universal |

- 先做别名归一化再检查组合；Android ABI 写法（arm64-v8a、armeabi-v7a、armeabi）只允许 android 使用。
- 不合法返回 1001，`errDlt` 例如 `invalid platform/arch combination ios/arm64, allowed arches for ios: universal`。
- add_version 直接拒绝；update_version 会对“已存记录 + 修改”整体校验，所以库里已有的非法组合记录不能再修改（返回 1001），只能删除后重新添加（迁移脚本会 WARN 列出）。upload_package 不带 platform/arch，不涉及。
- latest_version（客户端）不报错：ios 一律按 universal；其它平台只做别名归一化，非法组合（如 macos+armv7）实际上只会匹配 universal 记录。与 elephant-chat 的 `LookupArch` 一致。

其它值在 add/update 时返回 1001；客户端上报未知 arch 时只匹配 universal 记录。比较时库里未迁移的别名（如 `armeabi-v7a`）也按规范值匹配。
（Elephant 那边服务端白名单 armeabi-v7a/amd64 与客户端上报 armv7/x64 不一致，这里没有照搬。）

## 强制更新规则补充
- 只看最新那条：跳过的中间版本上的 `force` 不会继承；要让低于某版本的客户端必须更新，在最新记录上设 `min_version`。
- `revoked` 记录完全不参与候选（也不影响 force）；撤回最新版后自动回退到下一个 published 版本。
- `has_update=false` 时 `latest` 仍可能返回（客户端已是最新时就是它自己那版），客户端不要只靠 `latest != null` 判断。

## 接口
Chat API（默认 :10008）与 Admin API（:10009）均提供 `POST /application/latest_version`（无需 token）。其余在 Admin API，需 header `token: <adminToken>`、`operationID: <任意>`。
响应统一外壳：成功 `{"errCode":0,"errMsg":"","errDlt":"","data":{...}}`；失败 `{"errCode":1001,"errMsg":"ArgsError","errDlt":"原因"}`（重复 1003 DuplicateKeyError，不存在 1004 RecordNotFoundError，未带 token 1001 "token is empty"）。

| 路由 | 说明 |
|---|---|
| POST /application/latest_version | `{platform,arch,version,build}` → `{has_update,force,latest}` |
| POST /application/page_versions | `{platform:[],pagination:{pageNumber,showNumber}}` → `{total,versions[]}`（管理员） |
| POST /application/add_version | 扁平字段，返回 `{id}` |
| POST /application/update_version | `{id, url?, size?, sha256?, package_type?, text?, min_version?, force?, status?}`；每个字段既可写 `"text":"x"` 也可写 `"text":{"value":"x"}` |
| POST /application/revoke_version | `{id}`，等价于 status=revoked |
| POST /application/delete_version | `{id:[...]}` |
| POST /application/upload_package | multipart 字段 `file`（别名 `/application/upload`）→ `{url,size,sha256,filename,package_type,deduplicated}`（同 sha256+size 已存在时返回已有文件，`deduplicated=true`） |
| POST /application/check_package | 管理员，秒传检查 `{sha256,size,filename}` → `{exists:true,url,size,sha256,filename,package_type}` 或 `{exists:false}`，见下文 |
| POST /application/probe_url | `{url}` → `{size,sha256,filename}` |

## 上传与探测
- 上传沿用项目已有的"本地磁盘 + gin Static"机制（同申诉证据 `AppealStorage`），新增 `PackageStorage`：目录取自 `config/chat-api-admin.yml` 的 `appPackage.dir`（仓库默认 `./app_packages`，相对路径按项目根即配置目录的上一级解析，与启动方式/cwd 无关；也可写绝对路径），下载路径 `/application/files/...`（无需鉴权，供客户端下载；隐藏路径 404，见“秒传”）。流式写盘并同时计算 size、sha256，上限 1 GiB，仅允许 apk/exe/dmg/zip/deb/ipa/tar.gz，文件名消毒并放入随机目录。返回的 url 为绝对地址：取自 `appPackage.baseURL`（仓库默认留空；生产请配置为 admin API 对外地址，如 `https://<域名>/chat-admin-api`，下载地址形如 `<baseURL>/application/files/<随机目录>/<文件名>`）；该值留空时回退到请求的 `X-Forwarded-Proto/Host`（不含反向代理路径前缀）。不再使用环境变量。请持久化该目录。
- probe_url 防 SSRF：仅 http/https、禁止带账号；在拨号时检查解析后的实际 IP（回环、私网、链路本地含 169.254.169.254、CGNAT、组播、保留段全部拒绝，重定向后同样检查，防 DNS rebinding）；最多 5 次重定向；忽略环境代理；1 GiB 上限；总超时 5 分钟。流式计算 sha256，不落盘。

## 秒传（check_package，同文件不重传）
流程：后台先在本地算 sha256 和 size，调 `POST /application/check_package`；`exists=true` 时直接用返回的 url/size/sha256 填表，否则再走 upload_package。

- sha256 会先去空白并转小写，再校验必须是 64 位十六进制，否则返回 errCode 1001（ArgsError）；size 必须 > 0（否则也是 1001）。**sha256 和 size 都相同**才算命中；`filename` 只作参考，不参与匹配，返回的 filename 是已存文件的名字。
- 索引：每次 upload_package 存盘后写 `<appPackage.dir>/.index/<sha256>.json`（临时文件 + rename 原子写入），内容 `{path(相对 dir), size, sha256, filename, package_type, create_time(ms)}`。索引写失败不影响上传结果。
- 校验：查到索引后会 `os.Stat` 文件，文件不存在或大小变了即视为失效，删除该索引并返回 `exists=false`。仅请求的 size 不一致时不删索引。
- 上传去重：upload_package 写完新文件后，若同 sha256+size 的旧文件仍在，就删掉刚写的副本、返回旧文件（`deduplicated=true`）；旧文件已失效则保留新文件并重建索引。
- 补索引：admin-api 启动后在后台 goroutine 扫描一次目录（跳过以 `.` 开头的目录和文件，只处理支持的后缀，已索引的路径不重算哈希；同内容多份时只索引第一份，其余不删除），结束时打印 `app package index backfill done` 日志（scanned/indexed/skipped/failed）。不阻塞启动。
- 下载路由 `/application/files/*` 不再用 gin Static，改为自定义处理：路径中任一段以 `.` 开头（`.index`、临时文件、`..`）或请求目录时一律 404，`.index` 不会对外暴露。
- url 拼法与 upload_package 完全相同（baseURL + `/application/files/...`）。

```json
// 命中
{"sha256":"6652eabd1eceba46459c321bb97517f4b13a8c1d5c870274ea8e5b135181c6ec","size":9,"filename":"XueHua-1.2.0.dmg"}
{"errCode":0,"errMsg":"","errDlt":"","data":{"exists":true,"url":"https://<域名>/chat-admin-api/application/files/20261020/600104639dbdff5b/XueHua-1.2.0.dmg","size":9,"sha256":"6652eabd1eceba46459c321bb97517f4b13a8c1d5c870274ea8e5b135181c6ec","filename":"XueHua-1.2.0.dmg","package_type":"dmg"}}
// 未命中 / size 不一致
{"errCode":0,"errMsg":"","errDlt":"","data":{"exists":false}}
// 哈希格式错
{"errCode":1001,"errMsg":"ArgsError","errDlt":"sha256 must be 64 hex characters"}
```


## 迁移
服务启动时会自动删除 v1 的两个旧索引（`platform_1_version_1`、`latest_-1`）并建立新索引（`uniq_release`、`platform_status`），但**不会改写已有文档**。v1 文档没有 `arch/build/status` 字段：迁移前它们不会出现在 `latest_version` 结果里（只取 `status=published`），在 `page_versions` 里 `arch/status` 为空。
需要保留老记录时，部署后执行 `docs/migrations/application-v2.js`（mongosh 脚本，不会自动执行）：补 `arch=universal, build=0, status=published` 等默认值、删除 `latest/hot`，把 arch 别名改写为规范值（会撞唯一键的只打印 WARN 不改），并列出不是严格 `x.y.z` 的版本号供手工修正/删除。若老数据无保留价值，可用 `DROP=true` 直接删集合。**不要在生产自动执行，先在副本上验证。**
