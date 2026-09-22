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

package io.github.leonyoah.stx.proxy.adapter;

import java.util.List;
import java.util.Map;

/**
 * Pluggable SPI interface for SeaTunnel runtime engine adaptation. Isolates differences across
 * SeaTunnel 2.3.x, 3.0.x, and future versions.
 */
public interface SeaTunnelEngineAdapter {

    /** Unique target adapter version identifier (e.g. "2.3", "3.0"). */
    String getAdapterVersion();

    /** Checks if this adapter supports the given engine version. */
    boolean supports(EngineVersion version);

    /** Deserializes checkpoint binary into normalized CompletedCheckpointData. */
    CompletedCheckpointData deserializeCheckpoint(byte[] rawBytes, ClassLoader classLoader)
            throws Exception;

    /** Deserializes IMAP WAL records into key/value inspection entries. */
    Map<String, Object> inspectIMapWal(
            Map<String, Object> request, byte[] rawBytes, ClassLoader classLoader) throws Exception;

    /** Extracts and normalizes business progress / offsets for all source targets. */
    List<Map<String, Object>> inspectSources(
            Map<String, Object> request,
            ClassLoader classLoader,
            CompletedCheckpointData checkpointData,
            List<String> warnings)
            throws Exception;

    /** Extracts and normalizes 2PC transaction status for all sink targets. */
    List<Map<String, Object>> inspectSinks(
            Map<String, Object> request,
            ClassLoader classLoader,
            CompletedCheckpointData checkpointData)
            throws Exception;

    /** Returns engine capabilities supported by this adapter. */
    Map<String, Object> getEngineCapabilities();
}
