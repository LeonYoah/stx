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

import org.junit.jupiter.api.Test;

import java.util.LinkedHashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class LogStreamOffsetFormatterTest {

    @Test
    void shouldFormatPostgresLsn() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values(
                                "lsn", "123456789",
                                "lsn_commit", "123456780",
                                "txId", "42",
                                "ts_usec", "1729500000000000"),
                        "Postgres-CDC");

        assertEquals("LSN 位点", summary.getLabel());
        assertEquals("LSN 123456789 / txId 42", summary.getValue());
        assertEquals(1729500000000L, summary.getEventTimeMillis());
        assertTrue(summary.hasOffset());
    }

    @Test
    void shouldIgnorePostgresSentinelEventTime() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values(
                                "lsn", "73594272",
                                "ts_usec", "-9223372036854775808",
                                "txId", "767"),
                        "Postgres-CDC");

        assertEquals("LSN 位点", summary.getLabel());
        assertEquals("LSN 73594272 / txId 767", summary.getValue());
        assertNull(summary.getEventTimeMillis());
    }

    @Test
    void shouldUseCommitLsnWhenPostgresLsnIsMissing() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values("lsn_commit", "123456780"), "Postgres-CDC");

        assertEquals("LSN 位点", summary.getLabel());
        assertEquals("LSN 123456780", summary.getValue());
        assertNull(summary.getEventTimeMillis());
    }

    @Test
    void shouldFormatMysqlBinlog() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values(
                                "file", "mysql-bin.000001",
                                "pos", "1234",
                                "ts_sec", "1729500000"),
                        "MySQL-CDC");

        assertEquals("Binlog 位点", summary.getLabel());
        assertEquals("mysql-bin.000001:1234", summary.getValue());
        assertEquals(1729500000000L, summary.getEventTimeMillis());
    }

    @Test
    void shouldIncludeMysqlGtid() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values(
                                "file", "mysql-bin.000001",
                                "pos", "1234",
                                "gtids", "3e11fa47-71ca-11e1-9e33-c80aa9429562:1-10"),
                        "MySQL-CDC");

        assertEquals("Binlog 位点", summary.getLabel());
        assertTrue(summary.getValue().startsWith("mysql-bin.000001:1234 / GTID "));
        assertTrue(summary.getValue().contains("3e11fa47"));
    }

    @Test
    void shouldFormatOracleScn() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values(
                                "scn", "100",
                                "commit_scn", "200",
                                "lcr_position", "1"),
                        "Oracle-CDC");

        assertEquals("SCN 位点", summary.getLabel());
        assertEquals("SCN 200 / lcr 1", summary.getValue());
    }

    @Test
    void shouldFormatSqlServerLsn() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values(
                                "commit_lsn", "0000:0000:0001",
                                "change_lsn", "0000:0000:0002",
                                "event_serial_no", "3"),
                        "SQLServer-CDC");

        assertEquals("LSN 位点", summary.getLabel());
        assertEquals("LSN 0000:0000:0001 / changeLsn 0000:0000:0002 / seq 3", summary.getValue());
    }

    @Test
    void shouldFormatDb2Lsn() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values(
                                "commit_lsn", "0000:0000:0001",
                                "change_lsn", "0000:0000:0002"),
                        "DB2-CDC");

        assertEquals("LSN 位点", summary.getLabel());
        assertEquals("LSN 0000:0000:0001 / changeLsn 0000:0000:0002", summary.getValue());
    }

    @Test
    void shouldFormatMongoResumeToken() {
        long bsonTimestamp = 7600000000000000000L;
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values(
                                "resumeToken",
                                "{\"_data\":\"8267ABCD\"}",
                                "timestamp",
                                String.valueOf(bsonTimestamp)),
                        "MongoDB-CDC");

        assertEquals("ResumeToken 位点", summary.getLabel());
        assertTrue(summary.getValue().startsWith("resumeToken {\"_data\":\"8267ABCD\"}"));
        assertTrue(summary.getValue().contains("/ ts "));
        assertEquals((bsonTimestamp >>> 32) * 1000L, summary.getEventTimeMillis());
    }

    @Test
    void shouldFormatTimestampOnlyOffset() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values("timestamp", "7600000000000000000"), "MongoDB-CDC");

        assertEquals("时间戳位点", summary.getLabel());
        assertTrue(summary.getValue().startsWith("ts 7600000000000000000"));
        assertNotNull(summary.getEventTimeMillis());
    }

    @Test
    void shouldFormatDirectMillisecondTimestamp() {
        long millis = 1729500000000L;
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values("timestamp", String.valueOf(millis)), "Custom-CDC");

        assertEquals("时间戳位点", summary.getLabel());
        assertEquals(millis, summary.getEventTimeMillis());
    }

    @Test
    void shouldRecognizeOpenGaussAndKingbaseLsnLabel() {
        LogStreamOffsetFormatter.Summary opengauss =
                LogStreamOffsetFormatter.empty("OpenGauss-CDC");
        assertEquals("LSN 位点", opengauss.getLabel());

        LogStreamOffsetFormatter.Summary kingbase = LogStreamOffsetFormatter.empty("Kingbase-CDC");
        assertEquals("LSN 位点", kingbase.getLabel());
    }

    @Test
    void shouldFormatCamelCaseLsnAndChangeLsn() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values(
                                "commitLsn", "0000:0001",
                                "changeLsn", "0000:0002",
                                "eventSerialNo", "5"),
                        "SQLServer-CDC");

        assertEquals("LSN 位点", summary.getLabel());
        assertEquals("LSN 0000:0001 / changeLsn 0000:0002 / seq 5", summary.getValue());
    }

    @Test
    void shouldFormatTidbTsoOffset() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values("commit_ts", "438271928371928"), "TiDB-CDC");

        assertEquals("TSO 位点", summary.getLabel());
        assertEquals("TSO 438271928371928", summary.getValue());
    }

    @Test
    void shouldUseConnectorSpecificLabelWhenOffsetIsEmpty() {
        LogStreamOffsetFormatter.Summary postgres = LogStreamOffsetFormatter.empty("Postgres-CDC");
        assertEquals("LSN 位点", postgres.getLabel());
        assertEquals("-", postgres.getValue());
        assertFalse(postgres.hasOffset());

        LogStreamOffsetFormatter.Summary oracle = LogStreamOffsetFormatter.empty("Oracle-CDC");
        assertEquals("SCN 位点", oracle.getLabel());

        LogStreamOffsetFormatter.Summary mongo = LogStreamOffsetFormatter.empty("MongoDB-CDC");
        assertEquals("ResumeToken 位点", mongo.getLabel());

        LogStreamOffsetFormatter.Summary mysql = LogStreamOffsetFormatter.empty("MySQL-CDC");
        assertEquals("Binlog 位点", mysql.getLabel());
    }

    @Test
    void shouldFallBackToGenericLabelForUnknownOffset() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(values("unknown", "value"), "Custom-CDC");

        assertEquals("增量位点", summary.getLabel());
        assertEquals("-", summary.getValue());
        assertFalse(summary.hasOffset());
    }

    @Test
    void shouldPreferLsnOverBinlogKeys() {
        LogStreamOffsetFormatter.Summary summary =
                LogStreamOffsetFormatter.summarize(
                        values(
                                "lsn", "123",
                                "file", "mysql-bin.000001",
                                "pos", "1"),
                        "Custom-CDC");

        assertEquals("LSN 位点", summary.getLabel());
        assertEquals("LSN 123", summary.getValue());
    }

    private static Map<String, Object> values(String... keyValues) {
        Map<String, Object> values = new LinkedHashMap<>();
        for (int i = 0; i + 1 < keyValues.length; i += 2) {
            values.put(keyValues[i], keyValues[i + 1]);
        }
        return values;
    }
}
