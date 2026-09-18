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
package io.github.leonyoah.stx.proxy.service.config;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertTrue;

class JobConfigSupportServiceTest {

    @Test
    void missingConnectorShouldReturnActionablePreviewMessage() {
        RuntimeException cause =
                new RuntimeException(
                        "Plugin PluginIdentifier{engineType='seatunnel', pluginType='sink', pluginName='Http'} not found.");
        RuntimeException failure =
                new RuntimeException("Unable to create a sink for identifier 'Http'.", cause);

        String message =
                JobConfigSupportService.buildOfficialResolutionError("Sink", 0, "Http", failure);

        assertTrue(message.contains("Sink[0]-Http 连接器未安装"));
        assertTrue(message.contains("请先给当前集群安装 Http 连接器"));
    }
}
