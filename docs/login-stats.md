# Dashboard login statistics

`POST /statistic/login_user_count` (admin API, header `token: <admin chat token>`; `mw.CheckAdmin`)

Request: `{"start": <ms epoch>, "end": <ms epoch>}` (range is `[start, end)`; `start > end` is an error; max 3660 days).

Response `data`:
```json
{"loginCount": 3, "unloginCount": 117, "count": {"2026-10-13": 0, "2026-10-14": 2, "2026-10-15": 3}}
```
- `count`: day (`YYYY-MM-DD`, **Asia/Shanghai**, fixed +08:00) -> number of **distinct users** that logged in that day (rows of `user_login_record`, field `login_time`). Every day in the range is present; days without logins are `0`.
- `loginCount`: distinct users with any login in the range (not the sum of the daily numbers).
- `unloginCount`: registered users (`register` collection) minus `loginCount`, floored at 0.

## New users: `POST /statistic/new_user_count`

See `docs/new-user-stats.md` (`{"total", "date_count"}`, Asia/Shanghai days, zero-filled, `total` = users
registered in the range).

Login records (`user_login_record`) are written by: `Login` (account / phone / email, password or verify code),
QR login confirm, and `RegisterUser` with `autoLogin`. There is no separate BTC/web3 login or token-refresh path in this repo.
Login records written before this change are still counted (they always had `login_time`), but registrations with `autoLogin` were not recorded before; there is no backfill (see docs/user-times.md).
