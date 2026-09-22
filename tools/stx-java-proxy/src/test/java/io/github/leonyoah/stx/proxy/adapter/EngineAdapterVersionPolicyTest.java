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

import static org.junit.jupiter.api.Assertions.assertEquals;

class EngineAdapterVersionPolicyTest {

    @Test
    void testBuiltInVersionResolution() {
        EngineAdapterVersionPolicy policy = new EngineAdapterVersionPolicy();

        // 2.3.x resolves to 2.3
        assertEquals("2.3", policy.resolveAdapterVersion(EngineVersion.parse("2.3.13")));
        assertEquals("2.3", policy.resolveAdapterVersion(EngineVersion.parse("2.3.0")));

        // 3.0.x resolves to 3.0
        assertEquals("3.0", policy.resolveAdapterVersion(EngineVersion.parse("3.0.0")));
        assertEquals("3.0", policy.resolveAdapterVersion(EngineVersion.parse("3.0.1")));

        // Forward compatibility: future 3.x minor versions (e.g. 3.1.0, 3.2.0)
        // automatically reuse 3.0 adapter unless explicitly configured otherwise
        assertEquals("3.0", policy.resolveAdapterVersion(EngineVersion.parse("3.1.0")));
        assertEquals("3.0", policy.resolveAdapterVersion(EngineVersion.parse("3.2.5")));
    }

    @Test
    void testDynamicMappingAndForwardReuse() {
        EngineAdapterVersionPolicy policy = new EngineAdapterVersionPolicy();

        // Register custom rule: future version 4.0.* reuses 3.0 adapter
        policy.registerMapping("4.*", "3.0");
        assertEquals("3.0", policy.resolveAdapterVersion(EngineVersion.parse("4.0.0")));

        // Register custom override: 3.9.* maps to experimental
        policy.registerMapping("3.9.*", "experimental-3.9");
        assertEquals(
                "experimental-3.9", policy.resolveAdapterVersion(EngineVersion.parse("3.9.0")));
    }
}
