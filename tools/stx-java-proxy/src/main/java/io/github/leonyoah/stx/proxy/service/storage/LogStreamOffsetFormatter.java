/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package io.github.leonyoah.stx.proxy.service.storage;

import java.util.Locale;
import java.util.Map;

/**
 * Formats the connector-specific source offset stored in a CDC checkpoint.
 *
 * <p>SeaTunnel CDC connectors deliberately use different offset models: MySQL/TiDB store binlog
 * file/position, Postgres stores an LSN, Oracle stores an SCN, SQL Server/DB2 store commit/change
 * LSNs, and MongoDB stores a change-stream resume token. The checkpoint inspector must not label
 * all of them as binlog offsets.
 *
 * <p>SeaTunnel CDC 连接器的 offset 模型并不相同：MySQL/TiDB 是 binlog file/pos，Postgres 是 LSN， Oracle 是 SCN，SQL
 * Server/DB2 是 commit/change LSN，MongoDB 是 resume token。checkpoint 展示层 不能把它们统一标成 Binlog 位点。
 */
final class LogStreamOffsetFormatter {

    private static final int MAX_DETAIL_LENGTH = 64;

    private LogStreamOffsetFormatter() {}

    /**
     * Builds a display summary for one CDC source offset map.
     *
     * <p>The values map is the {@code startupOffset.values} map produced by the checkpoint
     * inspector. It may be empty when the source has not produced an offset yet; in that case the
     * label is still inferred from the connector name so the UI does not fall back to a
     * MySQL-specific label for Postgres/Oracle/MongoDB.
     */
    static Summary summarize(Map<String, ?> values, String pluginName) {
        String lsn =
                firstNonBlank(
                        values, "lsn", "lsn_commit", "last_commit_lsn", "commit_lsn", "commitLsn");
        String changeLsn = firstNonBlank(values, "change_lsn", "changeLsn");
        String txId = firstNonBlank(values, "txId", "tx_id");
        String eventSerialNo = firstNonBlank(values, "event_serial_no", "eventSerialNo");
        String xmin = firstNonBlank(values, "xmin");
        String scn = firstNonBlank(values, "commit_scn", "scn", "commitScn");
        String lcrPosition = firstNonBlank(values, "lcr_position", "lcrPosition");
        String resumeToken = firstNonBlank(values, "resumeToken", "resume_token");
        String timestamp = firstNonBlank(values, "timestamp");
        String file = firstNonBlank(values, "file");
        String pos = firstNonBlank(values, "pos");
        String gtids = firstNonBlank(values, "gtids", "gtid");
        String tso = firstNonBlank(values, "commit_ts", "start_ts", "resolved_ts", "tso");
        String tsSec = firstNonBlank(values, "ts_sec");
        String tsUsec = firstNonBlank(values, "ts_usec");

        Long eventTimeMillis = parseEventTimeMillis(tsUsec, tsSec, timestamp);

        if (lsn != null) {
            StringBuilder value = new StringBuilder("LSN ").append(lsn);
            if (changeLsn != null && !changeLsn.equals(lsn)) {
                value.append(" / changeLsn ").append(changeLsn);
            }
            if (txId != null) {
                value.append(" / txId ").append(txId);
            }
            if (eventSerialNo != null) {
                value.append(" / seq ").append(eventSerialNo);
            }
            if (xmin != null && txId == null && eventSerialNo == null) {
                value.append(" / xmin ").append(xmin);
            }
            return new Summary("LSN 位点", value.toString(), eventTimeMillis);
        }

        if (scn != null) {
            StringBuilder value = new StringBuilder("SCN ").append(scn);
            if (lcrPosition != null) {
                value.append(" / lcr ").append(lcrPosition);
            }
            return new Summary("SCN 位点", value.toString(), eventTimeMillis);
        }

        if (resumeToken != null) {
            StringBuilder value = new StringBuilder("resumeToken ").append(truncate(resumeToken));
            if (timestamp != null) {
                value.append(" / ts ").append(timestamp);
            }
            return new Summary("ResumeToken 位点", value.toString(), eventTimeMillis);
        }

        if (file != null || pos != null) {
            StringBuilder value = new StringBuilder();
            if (file != null) {
                value.append(file);
            }
            if (pos != null) {
                if (value.length() > 0) {
                    value.append(':');
                }
                value.append(pos);
            }
            if (gtids != null) {
                value.append(" / GTID ").append(truncate(gtids));
            }
            return new Summary("Binlog 位点", value.toString(), eventTimeMillis);
        }

        if (tso != null) {
            return new Summary("TSO 位点", "TSO " + tso, eventTimeMillis);
        }

        if (timestamp != null) {
            return new Summary("时间戳位点", "ts " + timestamp, eventTimeMillis);
        }

        return new Summary(defaultLabel(pluginName), "-", null);
    }

    static Summary empty(String pluginName) {
        return new Summary(defaultLabel(pluginName), "-", null);
    }

    private static String defaultLabel(String pluginName) {
        String normalized = pluginName == null ? "" : pluginName.toLowerCase(Locale.ROOT);
        if (normalized.contains("postgres")
                || normalized.contains("sqlserver")
                || normalized.contains("db2")
                || normalized.contains("opengauss")
                || normalized.contains("kingbase")
                || normalized.contains("gauss")) {
            return "LSN 位点";
        }
        if (normalized.contains("oracle")) {
            return "SCN 位点";
        }
        if (normalized.contains("mongo")) {
            return "ResumeToken 位点";
        }
        if (normalized.contains("mysql") || normalized.contains("tidb")) {
            return "Binlog 位点";
        }
        return "增量位点";
    }

    private static Long parseEventTimeMillis(String tsUsec, String tsSec, String timestamp) {
        if (tsUsec != null) {
            Long micros = parseLong(tsUsec);
            if (micros != null && micros > 0) {
                return micros / 1000L;
            }
        }
        if (tsSec != null) {
            Long seconds = parseLong(tsSec);
            if (seconds != null && seconds > 0) {
                return seconds * 1000L;
            }
        }
        if (timestamp != null) {
            Long rawTimestamp = parseLong(timestamp);
            if (rawTimestamp != null && rawTimestamp > 0) {
                // MongoDB 的 BsonTimestamp 把秒放在高 32 位，低 32 位是自增序号（秒数通常落在 2001~2065 年之间，约 1e9 ~ 3e9）。
                // MongoDB BsonTimestamp packs epoch seconds into the upper 32 bits (typically
                // between 1e9 and 3e9).
                long seconds = rawTimestamp >>> 32;
                if (seconds >= 1_000_000_000L && seconds < 3_000_000_000L) {
                    return seconds * 1000L;
                }
                // 若本身为 13 位毫秒级时间戳（如 1729500000000L）。
                // Direct 13-digit millisecond epoch timestamp.
                if (rawTimestamp >= 1_000_000_000_000L && rawTimestamp < 10_000_000_000_000L) {
                    return rawTimestamp;
                }
                // 若为 10 位秒级时间戳（如 1729500000L）。
                // Direct 10-digit second epoch timestamp.
                if (rawTimestamp >= 1_000_000_000L && rawTimestamp < 10_000_000_000L) {
                    return rawTimestamp * 1000L;
                }
                // 其他情况兜底：若高 32 位有正数值则尝试按秒折算，否则直接返回原数值。
                // Fallback: if upper bits have positive value, treat as seconds; otherwise return
                // as-is.
                if (seconds > 0) {
                    return seconds * 1000L;
                }
                return rawTimestamp;
            }
        }
        return null;
    }

    private static Long parseLong(String value) {
        if (value == null || value.isEmpty()) {
            return null;
        }
        try {
            return Long.parseLong(value.trim());
        } catch (NumberFormatException ignored) {
            return null;
        }
    }

    private static String firstNonBlank(Map<String, ?> values, String... keys) {
        if (values == null) {
            return null;
        }
        for (String key : keys) {
            Object raw = values.get(key);
            if (raw == null) {
                continue;
            }
            String value = String.valueOf(raw).trim();
            if (!value.isEmpty()) {
                return value;
            }
        }
        return null;
    }

    private static String truncate(String value) {
        if (value == null || value.length() <= MAX_DETAIL_LENGTH) {
            return value;
        }
        return value.substring(0, MAX_DETAIL_LENGTH) + "...";
    }

    /** One formatted CDC offset, including the event timestamp used for lag calculation. */
    static final class Summary {
        private final String label;
        private final String value;
        private final Long eventTimeMillis;

        Summary(String label, String value, Long eventTimeMillis) {
            this.label = label;
            this.value = value;
            this.eventTimeMillis = eventTimeMillis;
        }

        String getLabel() {
            return label;
        }

        String getValue() {
            return value;
        }

        Long getEventTimeMillis() {
            return eventTimeMillis;
        }

        boolean hasOffset() {
            return value != null && !value.isEmpty() && !"-".equals(value);
        }
    }
}
