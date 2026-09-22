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
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class EngineVersionTest {

    @Test
    void testParseAndMatches() {
        EngineVersion v2313 = EngineVersion.parse("2.3.13");
        assertEquals(2, v2313.getMajor());
        assertEquals(3, v2313.getMinor());
        assertEquals(13, v2313.getPatch());
        assertTrue(v2313.isVersion2x());
        assertFalse(v2313.isVersion3x());
        assertTrue(v2313.matches("2.3.*"));
        assertTrue(v2313.matches("2.*"));
        assertTrue(v2313.matches("2.3.13"));
        assertFalse(v2313.matches("3.*"));

        EngineVersion v300 = EngineVersion.parse("v3.0.0");
        assertEquals(3, v300.getMajor());
        assertEquals(0, v300.getMinor());
        assertEquals(0, v300.getPatch());
        assertTrue(v300.isVersion3x());
        assertFalse(v300.isVersion2x());
        assertTrue(v300.matches("3.0.*"));
        assertTrue(v300.matches("3.*"));
        assertFalse(v300.matches("2.3.*"));

        EngineVersion v310 = EngineVersion.parse("3.1.0-preview");
        assertEquals(3, v310.getMajor());
        assertEquals(1, v310.getMinor());
        assertTrue(v310.isVersion3x());
        assertTrue(v310.matches("3.*"));
        assertTrue(v310.matches("3.1.*"));
        assertFalse(v310.matches("3.0.*"));
    }

    @Test
    void testCompareTo() {
        EngineVersion v2 = EngineVersion.parse("2.3.13");
        EngineVersion v3 = EngineVersion.parse("3.0.0");
        EngineVersion v31 = EngineVersion.parse("3.1.0");

        assertTrue(v2.compareTo(v3) < 0);
        assertTrue(v3.compareTo(v31) < 0);
        assertTrue(v31.compareTo(v3) > 0);
    }
}
