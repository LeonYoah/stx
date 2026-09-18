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

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;

public class StreamingSourceDescriptorRegistry {

    public enum DecodeStrategy {
        CHANGE_STREAM_FACTORY,
        DEFAULT_SERIALIZER_TYPED,
        SINGLE_SPLIT_EMPTY
    }

    public static final class StreamingSourceDescriptor {
        private final String pluginName;
        private final DecodeStrategy decodeStrategy;
        private final String splitClassName;
        private final String enumeratorStateClassName;
        private final String factoryClassName;
        private final String projectorId;

        StreamingSourceDescriptor(
                String pluginName,
                DecodeStrategy decodeStrategy,
                String splitClassName,
                String enumeratorStateClassName,
                String factoryClassName,
                String projectorId) {
            this.pluginName = pluginName;
            this.decodeStrategy = decodeStrategy;
            this.splitClassName = splitClassName;
            this.enumeratorStateClassName = enumeratorStateClassName;
            this.factoryClassName = factoryClassName;
            this.projectorId = projectorId;
        }

        public String getPluginName() {
            return pluginName;
        }

        public DecodeStrategy getDecodeStrategy() {
            return decodeStrategy;
        }

        public String getSplitClassName() {
            return splitClassName;
        }

        public String getEnumeratorStateClassName() {
            return enumeratorStateClassName;
        }

        public String getFactoryClassName() {
            return factoryClassName;
        }

        public String getProjectorId() {
            return projectorId;
        }
    }

    private static final String CDC_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.cdc.base.source.split.IncrementalSplit";
    private static final String CDC_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.cdc.base.source.enumerator.state.HybridPendingSplitsState";
    private static final String TIDB_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.cdc.tidb.source.split.TiDBSourceSplit";
    private static final String TIDB_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.cdc.tidb.source.enumerator.TiDBSourceCheckpointState";
    private static final String KAFKA_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.kafka.source.KafkaSourceSplit";
    private static final String KAFKA_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.kafka.state.KafkaSourceState";
    private static final String PULSAR_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.pulsar.source.split.PulsarPartitionSplit";
    private static final String PULSAR_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.pulsar.source.enumerator.PulsarSplitEnumeratorState";
    private static final String ROCKETMQ_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.rocketmq.source.RocketMqSourceSplit";
    private static final String ROCKETMQ_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.rocketmq.source.RocketMqSourceState";
    private static final String RABBITMQ_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.rabbitmq.split.RabbitmqSplit";
    private static final String RABBITMQ_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.rabbitmq.split.RabbitmqSplitEnumeratorState";
    private static final String SLS_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.sls.source.SlsSourceSplit";
    private static final String SLS_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.sls.state.SlsSourceState";
    private static final String TABLESTORE_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.tablestore.source.TableStoreSourceSplit";
    private static final String TABLESTORE_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.tablestore.source.TableStoreSourceState";
    private static final String PAIMON_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.paimon.source.PaimonSourceSplit";
    private static final String PAIMON_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.paimon.source.PaimonSourceState";
    private static final String ICEBERG_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.iceberg.source.split.IcebergFileScanTaskSplit";
    private static final String ICEBERG_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.iceberg.source.enumerator.IcebergSplitEnumeratorState";
    private static final String FAKE_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.fake.source.FakeSourceSplit";
    private static final String FAKE_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.fake.state.FakeSourceState";
    private static final String SINGLE_SPLIT_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.common.source.SingleSplit";
    private static final String SINGLE_SPLIT_ENUMERATOR_STATE_CLASS =
            "org.apache.seatunnel.connectors.seatunnel.common.source.SingleSplitEnumeratorState";
    private static final String DEFAULT_PROJECTOR = "default-projector";
    private static final String CDC_PROJECTOR = "cdc-projector";
    private static final String KAFKA_PROJECTOR = "kafka-projector";
    private static final String PULSAR_PROJECTOR = "pulsar-projector";
    private static final String ROCKETMQ_PROJECTOR = "rocketmq-projector";
    private static final String RABBITMQ_PROJECTOR = "rabbitmq-projector";
    private static final String SLS_PROJECTOR = "sls-projector";
    private static final String TABLESTORE_PROJECTOR = "tablestore-projector";
    private static final String PAIMON_PROJECTOR = "paimon-projector";
    private static final String ICEBERG_PROJECTOR = "iceberg-projector";
    private static final String FAKE_PROJECTOR = "fake-projector";
    private static final String TIDB_PROJECTOR = "tidb-projector";
    private static final String SINGLE_SPLIT_PROJECTOR = "single-split-projector";

    private final Map<String, StreamingSourceDescriptor> descriptors;

    public StreamingSourceDescriptorRegistry() {
        Map<String, StreamingSourceDescriptor> items = new LinkedHashMap<>();
        register(
                items,
                "MySQL-CDC",
                DecodeStrategy.CHANGE_STREAM_FACTORY,
                CDC_SPLIT_CLASS,
                CDC_ENUMERATOR_STATE_CLASS,
                "org.apache.seatunnel.connectors.seatunnel.cdc.mysql.source.MySqlIncrementalSourceFactory",
                CDC_PROJECTOR);
        register(
                items,
                "Oracle-CDC",
                DecodeStrategy.CHANGE_STREAM_FACTORY,
                CDC_SPLIT_CLASS,
                CDC_ENUMERATOR_STATE_CLASS,
                "org.apache.seatunnel.connectors.seatunnel.cdc.oracle.source.OracleIncrementalSourceFactory",
                CDC_PROJECTOR);
        register(
                items,
                "Postgres-CDC",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                CDC_SPLIT_CLASS,
                CDC_ENUMERATOR_STATE_CLASS,
                null,
                CDC_PROJECTOR);
        register(
                items,
                "SQLServer-CDC",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                CDC_SPLIT_CLASS,
                CDC_ENUMERATOR_STATE_CLASS,
                null,
                CDC_PROJECTOR);
        register(
                items,
                "OpenGauss-CDC",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                CDC_SPLIT_CLASS,
                CDC_ENUMERATOR_STATE_CLASS,
                null,
                CDC_PROJECTOR);
        register(
                items,
                "MongoDB-CDC",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                CDC_SPLIT_CLASS,
                CDC_ENUMERATOR_STATE_CLASS,
                null,
                CDC_PROJECTOR);
        register(
                items,
                "TiDB-CDC",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                TIDB_SPLIT_CLASS,
                TIDB_ENUMERATOR_STATE_CLASS,
                null,
                TIDB_PROJECTOR);
        register(
                items,
                "Kafka",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                KAFKA_SPLIT_CLASS,
                KAFKA_ENUMERATOR_STATE_CLASS,
                null,
                KAFKA_PROJECTOR);
        register(
                items,
                "Pulsar",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                PULSAR_SPLIT_CLASS,
                PULSAR_ENUMERATOR_STATE_CLASS,
                null,
                PULSAR_PROJECTOR);
        register(
                items,
                "Rocketmq",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                ROCKETMQ_SPLIT_CLASS,
                ROCKETMQ_ENUMERATOR_STATE_CLASS,
                null,
                ROCKETMQ_PROJECTOR);
        register(
                items,
                "RabbitMQ",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                RABBITMQ_SPLIT_CLASS,
                RABBITMQ_ENUMERATOR_STATE_CLASS,
                null,
                RABBITMQ_PROJECTOR);
        register(
                items,
                "Sls",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                SLS_SPLIT_CLASS,
                SLS_ENUMERATOR_STATE_CLASS,
                null,
                SLS_PROJECTOR);
        register(
                items,
                "TableStore",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                TABLESTORE_SPLIT_CLASS,
                TABLESTORE_ENUMERATOR_STATE_CLASS,
                null,
                TABLESTORE_PROJECTOR);
        register(
                items,
                "Paimon",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                PAIMON_SPLIT_CLASS,
                PAIMON_ENUMERATOR_STATE_CLASS,
                null,
                PAIMON_PROJECTOR);
        register(
                items,
                "Iceberg",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                ICEBERG_SPLIT_CLASS,
                ICEBERG_ENUMERATOR_STATE_CLASS,
                null,
                ICEBERG_PROJECTOR);
        register(
                items,
                "FakeSource",
                DecodeStrategy.DEFAULT_SERIALIZER_TYPED,
                FAKE_SPLIT_CLASS,
                FAKE_ENUMERATOR_STATE_CLASS,
                null,
                FAKE_PROJECTOR);
        for (String name :
                new String[] {"Http", "Prometheus", "GraphQL", "Socket", "Web3j", "OpenMldb"}) {
            register(
                    items,
                    name,
                    DecodeStrategy.SINGLE_SPLIT_EMPTY,
                    SINGLE_SPLIT_CLASS,
                    SINGLE_SPLIT_ENUMERATOR_STATE_CLASS,
                    null,
                    SINGLE_SPLIT_PROJECTOR);
        }
        this.descriptors = Collections.unmodifiableMap(items);
    }

    public StreamingSourceDescriptor find(String pluginName) {
        if (pluginName == null) {
            return null;
        }
        return descriptors.get(normalize(pluginName));
    }

    private void register(
            Map<String, StreamingSourceDescriptor> items,
            String pluginName,
            DecodeStrategy decodeStrategy,
            String splitClassName,
            String enumeratorStateClassName,
            String factoryClassName,
            String projectorId) {
        items.put(
                normalize(pluginName),
                new StreamingSourceDescriptor(
                        pluginName,
                        decodeStrategy,
                        splitClassName,
                        enumeratorStateClassName,
                        factoryClassName,
                        projectorId));
    }

    private String normalize(String pluginName) {
        return pluginName.trim().toLowerCase(Locale.ROOT);
    }
}
