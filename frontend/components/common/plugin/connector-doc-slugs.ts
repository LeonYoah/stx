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

/**
 * Exact connector doc slugs from apache/seatunnel docs/en/connectors/{source,sink}.
 * 来自 apache/seatunnel 官方文档目录的精确 slug（大小写敏感）。
 *
 * Regenerated against seatunnel `dev` tree; used to avoid 404 doc links.
 * 对照 seatunnel `dev` 树生成，用于避免文档链接 404。
 */

export const DOC_SOURCE_SLUGS = new Set<string>([
  "Activemq", "Airtable", "AmazonDocumentDB", "AmazonDynamoDB", "AmazonSqs", "Asana",
  "AzureCosmosDB", "AzureEventHubs", "AzureQueueStorage", "BosFile", "Cassandra", "Clickhouse",
  "Cloudberry", "CosFile", "DB2", "DB2-CDC", "Databend", "Doris",
  "DuckDB", "Easysearch", "EdgeSocket", "Elasticsearch", "FacebookAds", "FakeSource",
  "Firebase", "Fluss", "FtpFile", "GcsFile", "Github", "Gitlab",
  "GoogleAds", "GoogleAnalytics4", "GoogleBigtable", "GooglePubSub", "GoogleSheets", "GraphQL",
  "Greenplum", "Hbase", "HdfsFile", "Hive", "HiveJdbc", "Http",
  "Http-Linear", "HugeGraph", "Iceberg", "InfluxDB", "Intercom", "IoTDB",
  "IoTDBv2", "Jdbc", "Jira", "Kafka", "Kingbase", "Klaviyo",
  "Kudu", "Lemlist", "LocalFile", "Mailchimp", "Maxcompute", "Milvus",
  "MongoDB", "MongoDB-CDC", "Mqtt", "MyHours", "MySQL-CDC", "Mysql",
  "Neo4j", "Notion", "ObsFile", "OceanBase", "OceanBase-CDC", "OneSignal",
  "OpenMldb", "Opengauss-CDC", "Oracle", "Oracle-CDC", "OssFile", "OssJindoFile",
  "Paimon", "PayPal", "Persistiq", "Phoenix", "PostHog", "PostgreSQL",
  "PostgreSQL-CDC", "Prometheus", "Pulsar", "Python", "Qdrant", "Rabbitmq",
  "Redis", "Redshift", "RocketMQ", "S3File", "SNMP", "Salesforce",
  "Sentry", "SftpFile", "Shopify", "Sls", "SmbFile", "Snowflake",
  "Socket", "Splunk", "SqlServer", "SqlServer-CDC", "StarRocks", "Stripe",
  "TDengine", "Tablestore", "TiDB-CDC", "TikTokAds", "Typeform", "Typesense",
  "Vertica", "Vitess-CDC", "Web3j", "WebSocket", "WooCommerce", "Zendesk",
]);

export const DOC_SINK_SLUGS = new Set<string>([
  "Activemq", "Aerospike", "Airtable", "AmazonDynamoDB", "AmazonSqs", "Assert",
  "AzureQueueStorage", "BigQuery", "BosFile", "Cassandra", "Clickhouse", "ClickhouseFile",
  "Cloudberry", "Console", "CosFile", "Couchbase", "DB2", "Databend",
  "Datahub", "DeepLake", "DingTalk", "Doris", "Druid", "DuckDB",
  "Easysearch", "Elasticsearch", "Email", "Enterprise-WeChat", "Feishu", "Firebase",
  "Fluss", "FtpFile", "GcsFile", "GoogleBigtable", "GoogleFirestore", "GooglePubSub",
  "GraphQL", "Greenplum", "Hbase", "HdfsFile", "Hive", "Http",
  "Hudi", "HugeGraph", "Iceberg", "InfluxDB", "IoTDB", "IoTDBv2",
  "Jdbc", "Kafka", "Kingbase", "Kudu", "Lance", "LocalFile",
  "Maxcompute", "Mem0", "Milvus", "MongoDB", "Mqtt", "Mysql",
  "NatsJetStream", "NebulaGraph", "Neo4j", "ObsFile", "OceanBase", "Oracle",
  "OssFile", "OssJindoFile", "Paimon", "Phoenix", "PostgreSql", "Prometheus",
  "Pulsar", "Qdrant", "Rabbitmq", "Redis", "Redshift", "RocketMQ",
  "S3-Redshift", "S3File", "SNMP", "Salesforce", "SelectDB-Cloud", "SensorsData",
  "Sentry", "SftpFile", "Slack", "Sls", "SmbFile", "Snowflake",
  "Socket", "SqlServer", "StarRocks", "Syslog", "TDengine", "Tablestore",
  "Typesense", "Vertica", "Zendesk",
]);

export const DOC_SINK_ONLY_SLUGS = new Set<string>([
  "Aerospike", "Assert", "BigQuery", "ClickhouseFile", "Console", "Couchbase",
  "Datahub", "DeepLake", "DingTalk", "Druid", "Email", "Enterprise-WeChat",
  "Feishu", "GoogleFirestore", "Hudi", "Lance", "Mem0", "NatsJetStream",
  "NebulaGraph", "PostgreSql", "S3-Redshift", "SelectDB-Cloud", "SensorsData", "Slack",
  "Syslog",
]);

export const DOC_SOURCE_ONLY_SLUGS = new Set<string>([
  "AmazonDocumentDB", "Asana", "AzureCosmosDB", "AzureEventHubs", "DB2-CDC", "EdgeSocket",
  "FacebookAds", "FakeSource", "Github", "Gitlab", "GoogleAds", "GoogleAnalytics4",
  "GoogleSheets", "HiveJdbc", "Http-Linear", "Intercom", "Jira", "Klaviyo",
  "Lemlist", "Mailchimp", "MongoDB-CDC", "MyHours", "MySQL-CDC", "Notion",
  "OceanBase-CDC", "OneSignal", "OpenMldb", "Opengauss-CDC", "Oracle-CDC", "PayPal",
  "Persistiq", "PostHog", "PostgreSQL", "PostgreSQL-CDC", "Python", "Shopify",
  "Splunk", "SqlServer-CDC", "Stripe", "TiDB-CDC", "TikTokAds", "Typeform",
  "Vitess-CDC", "Web3j", "WebSocket", "WooCommerce",
]);

/** Lowercase plugin/doc key → exact docs slug when naive casing fails.\n * 朴素大小写猜错时的覆盖表。 */
export const DOC_SLUG_OVERRIDES: Record<string, string> = {
  "amazondocumentdb": "AmazonDocumentDB",
  "amazondynamodb": "AmazonDynamoDB",
  "amazonsqs": "AmazonSqs",
  "azurecosmosdb": "AzureCosmosDB",
  "azureeventhubs": "AzureEventHubs",
  "azurequeuestorage": "AzureQueueStorage",
  "bigquery": "BigQuery",
  "bosfile": "BosFile",
  "cdc-mongodb": "MongoDB-CDC",
  "cdc-mysql": "MySQL-CDC",
  "cdc-oceanbase": "OceanBase-CDC",
  "cdc-opengauss": "Opengauss-CDC",
  "cdc-oracle": "Oracle-CDC",
  "cdc-postgres": "PostgreSQL-CDC",
  "cdc-postgresql": "PostgreSQL-CDC",
  "cdc-sqlserver": "SqlServer-CDC",
  "cdc-tidb": "TiDB-CDC",
  "clickhousefile": "ClickhouseFile",
  "cosfile": "CosFile",
  "db2": "DB2",
  "db2-cdc": "DB2-CDC",
  "deeplake": "DeepLake",
  "dingtalk": "DingTalk",
  "duckdb": "DuckDB",
  "edgesocket": "EdgeSocket",
  "enterprise-wechat": "Enterprise-WeChat",
  "facebookads": "FacebookAds",
  "fakesource": "FakeSource",
  "file-ftp": "FtpFile",
  "file-hdfs": "HdfsFile",
  "file-local": "LocalFile",
  "file-oss": "OssFile",
  "file-s3": "S3File",
  "file-sftp": "SftpFile",
  "ftpfile": "FtpFile",
  "gcsfile": "GcsFile",
  "googleads": "GoogleAds",
  "googleanalytics4": "GoogleAnalytics4",
  "googlebigtable": "GoogleBigtable",
  "googlefirestore": "GoogleFirestore",
  "googlepubsub": "GooglePubSub",
  "googlesheets": "GoogleSheets",
  "graphql": "GraphQL",
  "hdfsfile": "HdfsFile",
  "hivejdbc": "HiveJdbc",
  "http-linear": "Http-Linear",
  "hugegraph": "HugeGraph",
  "influxdb": "InfluxDB",
  "iotdb": "IoTDB",
  "iotdbv2": "IoTDBv2",
  "localfile": "LocalFile",
  "mongodb": "MongoDB",
  "mongodb-cdc": "MongoDB-CDC",
  "myhours": "MyHours",
  "mysql-cdc": "MySQL-CDC",
  "natsjetstream": "NatsJetStream",
  "nebulagraph": "NebulaGraph",
  "obsfile": "ObsFile",
  "oceanbase": "OceanBase",
  "oceanbase-cdc": "OceanBase-CDC",
  "onesignal": "OneSignal",
  "opengauss-cdc": "Opengauss-CDC",
  "openmldb": "OpenMldb",
  "oracle-cdc": "Oracle-CDC",
  "ossfile": "OssFile",
  "ossjindofile": "OssJindoFile",
  "paypal": "PayPal",
  "postgres": "PostgreSql",
  "postgresql-cdc": "PostgreSQL-CDC",
  "posthog": "PostHog",
  "rocketmq": "RocketMQ",
  "s3-redshift": "S3-Redshift",
  "s3file": "S3File",
  "selectdb-cloud": "SelectDB-Cloud",
  "sensorsdata": "SensorsData",
  "sftpfile": "SftpFile",
  "smbfile": "SmbFile",
  "snmp": "SNMP",
  "sqlserver": "SqlServer",
  "sqlserver-cdc": "SqlServer-CDC",
  "starrocks": "StarRocks",
  "tdengine": "TDengine",
  "tidb-cdc": "TiDB-CDC",
  "tiktokads": "TikTokAds",
  "vitess-cdc": "Vitess-CDC",
  "websocket": "WebSocket",
  "woocommerce": "WooCommerce",
};
