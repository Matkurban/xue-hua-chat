# New user statistics

`POST /statistic/new_user_count` (admin API, header `token: <admin chat token>`, `mw.CheckAdmin`).

Request: `{"start": <ms epoch>, "end": <ms epoch>}`; range is `[start, end)`. Errors: `start > end`, `start < 0`,
range longer than 3660 days. (The xue-hua-admin dashboard sends milliseconds.)

Response `data`:
```json
{"total": 7, "date_count": {"2026-10-13": 0, "2026-10-14": 2, "2026-10-15": 0, "2026-10-16": 5}}
```
- `date_count`: day (`YYYY-MM-DD`, **Asia/Shanghai** fixed +08:00) -> users registered that day.
  Every day of the range is present, `0` when nobody registered.
- `total`: users registered within `[start, end)` = sum of `date_count`. It is **not** all users.

Implementation (differs from elephant-chat, which counts `attribute.create_time` directly): the admin API still
calls the IM server (xue-hua-server) `POST /statistics/user/register` with an IM admin token and uses only its
`count` map. Since xue-hua-server a4e856e that map is grouped by Beijing date with every day in `[start, end)`
present. IM's `total` (all users) and `before` (users before `start`) are ignored; `total - before` must not be
used because it would also include users registered after `end`.
To stay correct against an older IM build, chat zero-fills missing days and drops keys outside the range
(`pkg/common/statsday.FillDays`) before summing `total`. (An older IM build groups by UTC date; those keys can only
be zero-filled, not re-bucketed, so deploy xue-hua-server >= a4e856e.)

Differences vs the old response: same field names (`total`, `date_count`); empty days are present as 0 and `total`
is the range count instead of all users.

Tests: `go test ./internal/api/admin ./pkg/common/statsday`.
