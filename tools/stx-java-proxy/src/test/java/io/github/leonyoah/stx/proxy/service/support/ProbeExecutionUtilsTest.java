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

package io.github.leonyoah.stx.proxy.service.support;

import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;

import java.util.LinkedHashMap;
import java.util.Map;

public class ProbeExecutionUtilsTest {

    @Test
    public void testRejectTooSmallTimeout() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put("probeTimeoutMs", 100L);

        ProxyException exception =
                Assertions.assertThrows(
                        ProxyException.class,
                        () ->
                                ProbeExecutionUtils.runWithTimeout(
                                        request, "Probe", "hint", () -> "ok"));
        Assertions.assertEquals(400, exception.getStatusCode());
        Assertions.assertTrue(exception.getMessage().contains("Probe timeout is too small"));
    }

    @Test
    public void testReturnTimeoutErrorWithHint() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put("probeTimeoutMs", 500L);

        ProxyException exception =
                Assertions.assertThrows(
                        ProxyException.class,
                        () ->
                                ProbeExecutionUtils.runWithTimeout(
                                        request,
                                        "Probe",
                                        "check test hint",
                                        () -> {
                                            Thread.sleep(5_000L);
                                            return "ok";
                                        }));
        Assertions.assertEquals(504, exception.getStatusCode());
        Assertions.assertTrue(exception.getMessage().contains("timed out"));
        Assertions.assertTrue(exception.getMessage().contains("check test hint"));
    }
}
