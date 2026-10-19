# 用户注册时间 / 最近登录时间

管理后台用户列表接口：`POST {API_ORIGIN}/chat/user/search/full`（chatAPI，不是 chat-admin-api），header `token: <admin 的 imToken/chatToken>`，`Content-Type: application/json`。

请求：`{"keyword":"","pagination":{"pageNumber":1,"showNumber":20},"genders":0,"normal":0}`

响应（字段总是返回，没有值时为 `0`）：
```json
{"errCode":0,"errMsg":"","errDlt":"","data":{"total":1,"users":[{"userID":"7381920465","account":"alice","email":"alice@example.com","nickname":"Alice","gender":1,"createTime":1760000000123,"lastLoginTime":1760500000456}]}}
```

| 字段 | 单位 | 含义 | 缺失时 |
|---|---|---|---|
| `createTime` | 毫秒 epoch (int64) | 注册时间：`attribute.create_time`，为空时回退 `register.create_time` | `0`（旧数据无法确定） |
| `lastLoginTime` | 毫秒 epoch (int64) | 最近一次登录：`user_login_record` 里该用户 `login_time` 的最大值 | `0`（从未有登录记录） |

`/user/find/full` 同样返回这两个字段。

## 根因
`UserFullInfo`（接口返回结构）根本没有这两个字段，`DbToPbUserFullInfo` 也没映射，前端读 `createTime`/`lastLoginTime` 永远是 undefined。数据库里有数据（`attribute.create_time`、`register.create_time`、`user_login_record.login_time`），只是没带出来。

## 写入位置
- 注册：`RegisterUser` / 管理员 `AddUserAccount` 同时写 `register.create_time` 和 `attribute.create_time`（time.Time，同一个值）。
- 登录：每次登录往 `user_login_record` 追加一条（`login_time`），入口：`Login`（密码/验证码，邮箱/手机/账号都走这里）、扫码登录 `qr_login.go`、以及本次新增的"注册后自动登录"（`AutoLogin`）。没有 token 刷新接口，也没有别的登录入口（BTC 账号走的是同一个 `Login`/注册流程）。
- 同时修了：`user_login_record` 的 `CountTotal` 按不存在的 `create_time` 过滤（统计错误），索引也建在不存在的 `create_time` 上；改为 `login_time`、(`user_id`,`login_time`)。

## 老数据
见 `docs/migrations/user-times-backfill.js`（默认只读，`APPLY=true` 才写）。注册时间已在读取时回退到 `register` 表，回填 attribute 只是可选；最近登录不伪造记录（会污染登录统计），老用户下次登录后就有值。**不要在生产自动执行。**
