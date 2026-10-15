# 全局时区设计 / Global Timezone Design

## 原则
1. **存储一律 UTC**：GORM `NowFunc = timex.Now`（UTC）；`db.UTCPlugin` 在写入前把模型/Map 中所有 `time.Time` 转为 UTC，并在查询/更新/删除前把 WHERE 参数中的 `time.Time` 转为 UTC。
   - SQLite：DATETIME 以文本比较，统一为 `...Z` 后字符串比较与时间比较一致（此前 `+08:00` 与 UTC 边界比较会漏数据，见 `HasScheduledJobInstanceInWindow`）。
   - MySQL：DSN `loc=UTC&time_zone='+00:00'`；PostgreSQL：`TimeZone=UTC`。
2. **区间查询**统一使用 `timex.Bound / BoundPtr / BoundAny`（插件是兜底）。
3. **API 输出** RFC3339（UTC，`Z`），`timex.FormatAPI`；**API 输入**必须带偏移，或同时提供 `tz`（IANA）参数（`timex.ParseAPITime`）。
4. **展示**：前端 `lib/timezone.ts`（Intl）按用户偏好时区格式化；用户时区为空=跟随浏览器。服务端渲染（诊断报告等）使用 `app.display_timezone`。
5. **Cron**：任务级 `timezone` 优先，缺省回退 `app.display_timezone`；自动巡检策略同样回退该值。
6. **SeaTunnel 无偏移时间**（引擎 REST、日志）：按 `STX_SEATUNNEL_TIMEZONE` 解析（Server 默认 display_timezone，Agent 默认主机本地时区），转为 UTC。

## 配置
| 键 | 默认 | 说明 |
|---|---|---|
| `app.display_timezone` / `STX_DISPLAY_TIMEZONE` | `Asia/Shanghai` | 系统默认展示时区；`STX_REPORT_TIMEZONE` 为兼容别名 |
| `database.legacy_timezone` | 进程本地时区 | 仅 MySQL 历史数据迁移使用 |
| `STX_SEATUNNEL_TIMEZONE` | Server: display tz；Agent: 主机时区 | 解析 SeaTunnel 无偏移时间 |
| 用户 `timezone`（PUT /api/v1/auth/profile） | 空 | 空=跟随浏览器 |

## 历史数据迁移
启动时 `MigrateTimeColumnsToUTC` 执行一次（标记表 `stx_schema_flags`，name=`time_columns_utc_v1`）：
- SQLite：`strftime('%Y-%m-%dT%H:%M:%fZ', col)` 重写所有非 `Z` 结尾且可解析的文本时间（保留绝对时刻，精度毫秒）；不可解析值不变。
- MySQL：`CONVERT_TZ(col, <legacy offset>, '+00:00')`，偏移取 `database.legacy_timezone`（缺省进程本地时区）当前偏移；含夏令时的时区请先确认。
- PostgreSQL：`timestamptz` 已是绝对时刻，仅写标记。

**升级前请备份**：SQLite 停服后复制 `data/stx.db*`；MySQL `mysqldump`。迁移在单个事务中执行，失败会阻止启动。
回滚：恢复备份（旧版本不识别 UTC 数据中的 `Z`/偏移差异，回滚后时间会偏移 8 小时）。

## 已知限制
- SQLite 文本比较在**同一秒内**的亚秒顺序可能不精确（RFC3339Nano 会裁剪末尾 0）。
