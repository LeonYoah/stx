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

import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class SeaTunnelEngineAdapterRegistryTest {

    @Test
    void testRegistryResolvesAdapters() {
        SeaTunnelEngineAdapterRegistry registry = SeaTunnelEngineAdapterRegistry.getInstance();

        SeaTunnelEngineAdapter adapter23 = registry.getAdapter("2.3.13");
        assertNotNull(adapter23);
        assertEquals("2.3", adapter23.getAdapterVersion());

        SeaTunnelEngineAdapter adapter30 = registry.getAdapter("3.0.0");
        assertNotNull(adapter30);
        assertEquals("3.0", adapter30.getAdapterVersion());

        // Forward compatibility check: 3.1.0 auto-routes to 3.0
        SeaTunnelEngineAdapter adapter31 = registry.getAdapter("3.1.0");
        assertNotNull(adapter31);
        assertEquals("3.0", adapter31.getAdapterVersion());

        Map<String, Object> caps30 = adapter30.getEngineCapabilities();
        assertEquals("3.0", caps30.get("adapterVersion"));
        assertTrue((Boolean) caps30.get("incrementalCheckpoint"));
        assertTrue((Boolean) caps30.get("tableLevelState"));
    }
}
