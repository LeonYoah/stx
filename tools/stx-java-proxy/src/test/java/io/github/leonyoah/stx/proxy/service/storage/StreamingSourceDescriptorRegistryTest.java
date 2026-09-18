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

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

class StreamingSourceDescriptorRegistryTest {

    @Test
    void shouldResolveKnownDescriptorsCaseInsensitively() {
        StreamingSourceDescriptorRegistry registry = new StreamingSourceDescriptorRegistry();

        StreamingSourceDescriptorRegistry.StreamingSourceDescriptor mysql =
                registry.find("mysql-cdc");
        StreamingSourceDescriptorRegistry.StreamingSourceDescriptor kafka = registry.find("Kafka");
        StreamingSourceDescriptorRegistry.StreamingSourceDescriptor http = registry.find("HTTP");

        assertNotNull(mysql);
        assertEquals(
                StreamingSourceDescriptorRegistry.DecodeStrategy.CHANGE_STREAM_FACTORY,
                mysql.getDecodeStrategy());
        assertNotNull(kafka);
        assertEquals(
                StreamingSourceDescriptorRegistry.DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                kafka.getDecodeStrategy());
        assertNotNull(http);
        assertEquals(
                StreamingSourceDescriptorRegistry.DecodeStrategy.SINGLE_SPLIT_EMPTY,
                http.getDecodeStrategy());
    }
}
