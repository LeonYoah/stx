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
 * Connector Quick Helper
 * 连接器快速辅助工具 - 提供纯前端零运行时依赖的 HOCON 生成、文档直达与语义图标映射
 *
 * Pure-frontend, zero-runtime-dependency helper for HOCON generation,
 * official docs navigation, and semantic icon classification.
 */

import {
  Activity,
  Cable,
  DatabaseZap,
  HardDrive,
  Layers,
  Radio,
  Search,
  Shuffle,
  type LucideIcon,
} from 'lucide-react';
import {DEFAULT_SEATUNNEL_DOC_VERSION} from '@/lib/seatunnel-version';

/**
 * Known CDC plugin mappings
 * 已知 CDC 插件命名映射
 */
const CDC_NAME_MAP: Record<string, string> = {
  'cdc-mysql': 'MySQL-CDC',
  'cdc-oracle': 'Oracle-CDC',
  'cdc-postgres': 'Postgres-CDC',
  'cdc-sqlserver': 'SqlServer-CDC',
  'cdc-opengauss': 'OpenGauss-CDC',
  'cdc-tidb': 'TiDB-CDC',
  'cdc-oceanbase': 'OceanBase-CDC',
  'cdc-mongodb': 'MongoDB-CDC',
};

/**
 * Special casing for SeaTunnel plugin identifiers
 * SeaTunnel 插件特定大小写标识映射
 */
const SPECIAL_IDENTIFIERS: Record<string, string> = {
  jdbc: 'Jdbc',
  kafka: 'Kafka',
  clickhouse: 'Clickhouse',
  doris: 'Doris',
  starrocks: 'StarRocks',
  amazondynamodb: 'AmazonDynamoDB',
  amazonsqs: 'AmazonSQS',
  activemq: 'ActiveMQ',
  rabbitmq: 'RabbitMQ',
  elasticsearch: 'Elasticsearch',
  easysearch: 'Easysearch',
  fake: 'Fake',
  assert: 'Assert',
  console: 'Console',
  email: 'Email',
  http: 'Http',
  redis: 'Redis',
  pulsar: 'Pulsar',
  rocketmq: 'Rocketmq',
  kudu: 'Kudu',
  iceberg: 'Iceberg',
  hudi: 'Hudi',
  hive: 'Hive',
  hbase: 'Hbase',
  cassandra: 'Cassandra',
  influxdb: 'InfluxDB',
  neo4j: 'Neo4j',
  iotdb: 'IoTDB',
  'file-ftp': 'FtpFile',
  'file-sftp': 'SftpFile',
  'file-s3': 'S3File',
  'file-oss': 'OssFile',
  'file-hdfs': 'HdfsFile',
  'file-local': 'LocalFile',
  'seatunnel-transforms-v2': 'Transform',
};

/**
 * Common JDBC database driver profiles
 * 常用 JDBC 场景驱动与连接配置
 */
const JDBC_PROFILE_TEMPLATES: Record<
  string,
  {driver: string; url: string; note: string}
> = {
  mysql: {
    driver: 'com.mysql.cj.jdbc.Driver',
    url: 'jdbc:mysql://localhost:3306/your_database',
    note: 'MySQL 8.x/5.7',
  },
  postgresql: {
    driver: 'org.postgresql.Driver',
    url: 'jdbc:postgresql://localhost:5432/your_database',
    note: 'PostgreSQL',
  },
  oracle: {
    driver: 'oracle.jdbc.OracleDriver',
    url: 'jdbc:oracle:thin:@localhost:1521:xe',
    note: 'Oracle Database',
  },
  sqlserver: {
    driver: 'com.microsoft.sqlserver.jdbc.SQLServerDriver',
    url: 'jdbc:sqlserver://localhost:1433;databaseName=your_database',
    note: 'Microsoft SQL Server',
  },
  dameng: {
    driver: 'dm.jdbc.driver.DmDriver',
    url: 'jdbc:dm://localhost:5236',
    note: 'Dameng (DM)',
  },
  clickhouse: {
    driver: 'com.clickhouse.jdbc.ClickHouseDriver',
    url: 'jdbc:clickhouse://localhost:8123/default',
    note: 'ClickHouse JDBC',
  },
  doris: {
    driver: 'com.mysql.cj.jdbc.Driver',
    url: 'jdbc:mysql://localhost:9030/your_database',
    note: 'Apache Doris FE',
  },
  starrocks: {
    driver: 'com.mysql.cj.jdbc.Driver',
    url: 'jdbc:mysql://localhost:9030/your_database',
    note: 'StarRocks FE',
  },
  sqlite: {
    driver: 'org.sqlite.JDBC',
    url: 'jdbc:sqlite:/path/to/database.db',
    note: 'SQLite',
  },
};

/**
 * Convert raw plugin name into standard PascalCase
 * 将原始插件名称转换为首字母大写格式
 */
function toPascalCase(str: string): string {
  if (!str) {
    return '';
  }
  return str
    .split(/[-_]+/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join('');
}

/**
 * Resolve canonical HOCON plugin identifier
 * 解析规范的 HOCON 插件标识符
 *
 * @param pluginName - Raw plugin name (e.g., 'cdc-mysql', 'jdbc') / 原始插件名
 * @returns Canonical HOCON identifier (e.g., 'MySQL-CDC', 'Jdbc') / 规范标识符
 */
export function getConnectorHoconIdentifier(pluginName: string): string {
  const normalized = (pluginName || '').trim().toLowerCase();
  if (CDC_NAME_MAP[normalized]) {
    return CDC_NAME_MAP[normalized];
  }
  if (SPECIAL_IDENTIFIERS[normalized]) {
    return SPECIAL_IDENTIFIERS[normalized];
  }
  if (normalized.startsWith('cdc-')) {
    const suffix = normalized.slice(4);
    return `${toPascalCase(suffix)}-CDC`;
  }
  return toPascalCase(normalized);
}

/**
 * Determine connector capabilities (Source vs Sink)
 * 推导连接器支持能力（Source 与 Sink）
 *
 * @param pluginName - Plugin identifier name / 插件名称
 * @param category - Declared category / 声明的分类
 */
export function getConnectorCapability(
  pluginName: string,
  category?: string,
): {
  supportsSource: boolean;
  supportsSink: boolean;
  isTransform: boolean;
} {
  const normalized = (pluginName || '').trim().toLowerCase();

  if (category === 'transform' || normalized.includes('transform')) {
    return {supportsSource: false, supportsSink: false, isTransform: true};
  }

  // CDC connectors are source only / CDC 连接器纯作为输入源
  if (normalized.startsWith('cdc-')) {
    return {supportsSource: true, supportsSink: false, isTransform: false};
  }

  // Pure sink connectors / 纯输出端连接器
  if (
    normalized === 'assert' ||
    normalized === 'console' ||
    normalized === 'email' ||
    normalized === 'sink-test'
  ) {
    return {supportsSource: false, supportsSink: true, isTransform: false};
  }

  // Pure source connectors / 纯输入端连接器
  if (normalized === 'fake') {
    return {supportsSource: true, supportsSink: false, isTransform: false};
  }

// Default: most SeaTunnel connectors support both Source and Sink
  // 默认：绝大多数 SeaTunnel 连接器均兼具输入与输出能力
  return {supportsSource: true, supportsSink: true, isTransform: false};
}

/**
 * Connector processing execution mode
 * 连接器数据处理执行模式：实时流式或离线批处理
 */
export type ConnectorProcessingMode = 'realtime' | 'offline';

/**
 * Determine supported processing modes for a connector (Real-time vs Offline)
 * 推导连接器支持的数据处理模式（实时流式与离线批处理）
 *
 * @param pluginName - Plugin identifier name / 插件标识符
 * @param category - Declared category / 声明分类
 * @returns Array of supported modes ('realtime', 'offline') / 支持的模式列表
 */
export function getConnectorProcessingModes(
  pluginName: string,
  category?: string,
): ConnectorProcessingMode[] {
  const normalized = (pluginName || '').trim().toLowerCase();

  // 1. Transform plugins support both streaming and batch
  // 1. 转换类插件（Transform）同时天然支持流处理与批处理
  if (category === 'transform' || normalized.includes('transform')) {
    return ['realtime', 'offline'];
  }

  // 2. CDC connectors are dedicated realtime streaming change data capture
  // 2. CDC 连接器为专用实时变更捕获流
  if (normalized.startsWith('cdc-') || normalized.endsWith('-cdc')) {
    return ['realtime'];
  }

  // 3. Message queues and streaming brokers (dual-capability: Kafka, Pulsar, RocketMQ)
  // 3. 消息队列与流式消息总线（部分兼备流读写与离线微批消费）
  if (
    normalized === 'kafka' ||
    normalized === 'pulsar' ||
    normalized === 'rocketmq' ||
    normalized === 'redis'
  ) {
    return ['realtime', 'offline'];
  }

  // 4. Pure streaming queues and protocols
  // 4. 纯实时流式消息队列与协议网络源
  if (
    normalized === 'rabbitmq' ||
    normalized === 'activemq' ||
    normalized.includes('sqs') ||
    normalized === 'socket' ||
    normalized === 'http' ||
    normalized === 'fluss' ||
    normalized === 'mqtt' ||
    normalized === 'websocket'
  ) {
    return ['realtime'];
  }

  // 5. Lakehouses: support realtime streaming sync and offline batch
  // 5. 现代流批一体湖仓：同时支持实时增量摄入与离线全量批处理
  if (
    normalized === 'iceberg' ||
    normalized === 'paimon' ||
    normalized === 'hudi'
  ) {
    return ['realtime', 'offline'];
  }

  // 6. Modern analytical data warehouses: support realtime stream load and offline batch
  // 6. 现代实时分析型数仓：兼具高频实时流写入 (Stream Load) 与离线批量同步
  if (
    normalized === 'clickhouse' ||
    normalized === 'doris' ||
    normalized === 'starrocks' ||
    normalized === 'selectdb'
  ) {
    return ['offline', 'realtime'];
  }

  // 7. Testing, virtual and debug connectors
  // 7. 虚拟测试与调试类连接器：流批皆可模拟
  if (
    normalized === 'fake' ||
    normalized === 'console' ||
    normalized === 'assert' ||
    normalized === 'sink-test'
  ) {
    return ['realtime', 'offline'];
  }

  // 8. Default: Relational databases, files, object storage, NoSQL, and Big Data warehouses are offline batch
  // 8. 默认：关系型数据库 (JDBC/MySQL/Pg等)、各类文件系统/对象存储 (S3/OSS/HDFS)、NoSQL 与传统大数据仓均为离线批处理
  return ['offline'];
}

/**
 * Check if a connector matches the specified category filter
 * 校验连接器是否匹配指定的分类过滤条件
 *
 * @param pluginName - Plugin name / 插件名称
 * @param filterMode - Category filter value ('all', 'realtime', 'offline', 'connector') / 分类过滤值
 * @param category - Declared category / 声明分类
 */
export function matchesConnectorProcessingMode(
  pluginName: string,
  filterMode: string,
  category?: string,
): boolean {
  if (!filterMode || filterMode === 'all' || filterMode === 'connector') {
    return true;
  }
  const modes = getConnectorProcessingModes(pluginName, category);
  return modes.includes(filterMode as ConnectorProcessingMode);
}

/**
 * Generate HOCON task configuration snippet
 * 生成 SeaTunnel HOCON 任务配置样例
 *
 * @param params - Generation options / 生成参数
 * @returns Formatted HOCON string / 格式化后的 HOCON 字符串
 */
export function generateHoconSnippet(params: {
  pluginName: string;
  mode: 'source' | 'sink';
  selectedProfileKeys?: string[];
}): string {
  const {pluginName, mode, selectedProfileKeys = []} = params;
  const hoconName = getConnectorHoconIdentifier(pluginName);
  const normalizedName = pluginName.toLowerCase().trim();

  // 1. Specific template for JDBC connectors with dynamic profile linkage
  // 1. JDBC 连接器专属模板：与当前所选 Profile 场景动态联动
  if (normalizedName === 'jdbc') {
    const matchedProfile = selectedProfileKeys.find(
      (key) => JDBC_PROFILE_TEMPLATES[key.toLowerCase()],
    );
    const profileConfig = matchedProfile
      ? JDBC_PROFILE_TEMPLATES[matchedProfile.toLowerCase()]
      : JDBC_PROFILE_TEMPLATES.mysql;

    if (mode === 'source') {
      return `source {
  Jdbc {
    result_table_name = "jdbc_source_table"
    url = "${profileConfig.url}"
    driver = "${profileConfig.driver}"
    user = "your_username"
    password = "your_password"
    query = "select * from your_table limit 100"
    # fetch_size = 1024
    # partition_column = "id"
    # partition_num = 4
  }
}`;
    }

    return `sink {
  Jdbc {
    source_table_name = "input_table"
    url = "${profileConfig.url}"
    driver = "${profileConfig.driver}"
    user = "your_username"
    password = "your_password"
    query = "insert into target_table (id, name) values (?, ?)"
    # batch_size = 1000
    # primary_keys = ["id"]
  }
}`;
  }

  // 2. MySQL-CDC and other CDC connectors
  // 2. MySQL-CDC 与其他 CDC 变更捕获连接器
  if (normalizedName.startsWith('cdc-')) {
    return `source {
  ${hoconName} {
    result_table_name = "${normalizedName}_cdc"
    hostname = "127.0.0.1"
    port = 3306
    username = "root"
    password = "your_password"
    database-name = "your_database"
    table-name = "your_database.*"
    # startup.mode = "INITIAL"
    # exactly_once = "true"
  }
}`;
  }

  // 3. Kafka connector
  // 3. Kafka 消息队列连接器
  if (normalizedName === 'kafka') {
    if (mode === 'source') {
      return `source {
  Kafka {
    result_table_name = "kafka_source"
    topic = "your_topic"
    bootstrap.servers = "localhost:9092"
    consumer.group = "seatunnel_group"
    format = "json"
    schema = {
      fields {
        id = "bigint"
        name = "string"
      }
    }
  }
}`;
    }
    return `sink {
  Kafka {
    source_table_name = "input_table"
    topic = "target_topic"
    bootstrap.servers = "localhost:9092"
    format = "json"
    semantics = "NON_EXACTLY_ONCE"
  }
}`;
  }

  // 4. ClickHouse connector
  // 4. ClickHouse 分析数仓连接器
  if (normalizedName === 'clickhouse') {
    if (mode === 'source') {
      return `source {
  Clickhouse {
    result_table_name = "clickhouse_source"
    host = "localhost:8123"
    database = "default"
    sql = "select * from your_table"
    username = "default"
    password = ""
  }
}`;
    }
    return `sink {
  Clickhouse {
    source_table_name = "input_table"
    host = "localhost:8123"
    database = "default"
    table = "target_table"
    username = "default"
    password = ""
  }
}`;
  }

  // 5. Doris / StarRocks connectors
  // 5. Doris / StarRocks 现代分析型数据仓库
  if (normalizedName === 'doris' || normalizedName === 'starrocks') {
    const isDoris = normalizedName === 'doris';
    const identifier = isDoris ? 'Doris' : 'StarRocks';
    return `sink {
  ${identifier} {
    source_table_name = "input_table"
    node_urls = ["localhost:8030"]
    username = "root"
    password = ""
    database = "your_database"
    table = "target_table"
    # batch_max_rows = 100000
    # batch_max_bytes = 104857600
  }
}`;
  }

  // 6. Generic source/sink fallback skeleton
  // 6. 通用输入与输出骨架兜底
  if (mode === 'source') {
    return `source {
  ${hoconName} {
    result_table_name = "${normalizedName}_source"
    # 请参考官方文档补充连接器详细参数 / Add connector parameters here
  }
}`;
  }

  return `sink {
  ${hoconName} {
    source_table_name = "input_table"
    # 请参考官方文档补充连接器详细参数 / Add connector parameters here
  }
}`;
}

/**
 * Official connector icon asset mapping (verified against seatunnel/docs/images/icons)
 * 官方连接器图标资源映射 (来源于 Apache SeaTunnel 官方文档与网站静态资源)
 */
const OFFICIAL_ICON_MAP: Record<string, string> = {
  mysql: '/icons/connectors/MySQL.svg',
  'mysql-cdc': '/icons/connectors/mysql-cdc.svg',
  'cdc-mysql': '/icons/connectors/mysql-cdc.svg',
  postgres: '/icons/connectors/PostgreSQL.svg',
  postgresql: '/icons/connectors/PostgreSQL.svg',
  'postgres-cdc': '/icons/connectors/postgresql-cdc.svg',
  'postgresql-cdc': '/icons/connectors/postgresql-cdc.svg',
  'cdc-postgres': '/icons/connectors/postgresql-cdc.svg',
  oracle: '/icons/connectors/Oracle.svg',
  'oracle-cdc': '/icons/connectors/oracle-cdc.svg',
  'cdc-oracle': '/icons/connectors/oracle-cdc.svg',
  sqlserver: '/icons/connectors/sqlserver.svg',
  'sqlserver-cdc': '/icons/connectors/sqlserver.svg',
  'cdc-sqlserver': '/icons/connectors/sqlserver.svg',
  clickhouse: '/icons/connectors/Clickhouse.png',
  doris: '/icons/connectors/Doris.svg',
  starrocks: '/icons/connectors/StarRocks.svg',
  kafka: '/icons/connectors/Kafka.svg',
  iceberg: '/icons/connectors/iceberg.svg',
  elasticsearch: '/icons/connectors/Elasticsearch.png',
  redis: '/icons/connectors/Redis.svg',
  mongodb: '/icons/connectors/MongoDB.svg',
  'mongodb-cdc': '/icons/connectors/MongoDB.svg',
  'cdc-mongodb': '/icons/connectors/MongoDB.svg',
  paimon: '/icons/connectors/Paimon.svg',
  pulsar: '/icons/connectors/Pulsar.svg',
  rocketmq: '/icons/connectors/RocketMQ.svg',
  rabbitmq: '/icons/connectors/Rabbitmq.svg',
  jdbc: '/icons/connectors/JDBC.svg',
  http: '/icons/connectors/Http.svg',
  'file-local': '/icons/connectors/LocalFile.svg',
  localfile: '/icons/connectors/LocalFile.svg',
  'file-hdfs': '/icons/connectors/hdfs.svg',
  hdfs: '/icons/connectors/hdfs.svg',
  'file-s3': '/icons/connectors/S3File.svg',
  s3: '/icons/connectors/S3File.svg',
  'file-sftp': '/icons/connectors/Sftp.svg',
  sftp: '/icons/connectors/Sftp.svg',
  'file-ftp': '/icons/connectors/FtpFile.svg',
  ftp: '/icons/connectors/FtpFile.svg',
  'file-obs': '/icons/connectors/ObsFile.png',
  obs: '/icons/connectors/ObsFile.png',
  hive: '/icons/connectors/Hive.svg',
  hbase: '/icons/connectors/Hbase.svg',
  iotdb: '/icons/connectors/IoTDB.svg',
  influxdb: '/icons/connectors/InfluxDB.svg',
  neo4j: '/icons/connectors/Neo4j.svg',
  milvus: '/icons/connectors/Milvus.svg',
  qdrant: '/icons/connectors/Qdrant.svg',
  snowflake: '/icons/connectors/Snowflake.svg',
  tdengine: '/icons/connectors/TDengine.svg',
  tablestore: '/icons/connectors/Tablestore.svg',
  cassandra: '/icons/connectors/Cassandra.png',
  greenplum: '/icons/connectors/Greenplum.svg',
  kingbase: '/icons/connectors/Kingbase.svg',
  oceanbase: '/icons/connectors/OceanBase.svg',
  'cdc-oceanbase': '/icons/connectors/OceanBase.svg',
  phoenix: '/icons/connectors/Phoenix.svg',
  maxcompute: '/icons/connectors/Maxcompute.svg',
  typesense: '/icons/connectors/Typesense.png',
  kudu: '/icons/connectors/Kudu.png',
  amazondynamodb: '/icons/connectors/AmazonDynamoDB.svg',
  gitlab: '/icons/connectors/Gitlab.svg',
  github: '/icons/connectors/Github.png',
  jira: '/icons/connectors/Jira.svg',
  notion: '/icons/connectors/Notion.svg',
};

/**
 * Resolve official connector icon path
 * 解析官方连接器图标静态路径
 *
 * @param pluginName - Plugin name / 插件名称
 */
export function getConnectorOfficialIconUrl(pluginName: string): string | null {
  if (!pluginName) {
    return null;
  }
  const clean = pluginName.toLowerCase().trim().replace(/^connector-/, '');
  if (OFFICIAL_ICON_MAP[clean]) {
    return OFFICIAL_ICON_MAP[clean];
  }
  for (const [key, path] of Object.entries(OFFICIAL_ICON_MAP)) {
    if (clean === key || clean.endsWith(`-${key}`) || clean.startsWith(`${key}-`)) {
      return path;
    }
  }
  return null;
}

/**
 * Exact SeaTunnel documentation slug mapping (verified against seatunnel/docs)
 * SeaTunnel 官方文档路径映射（已与 seatunnel 源码目录核对验证）
 *
 * @param pluginName - Plugin name / 插件名称
 * @param mode - Connector direction / 连接器方向
 */
export function getConnectorDocSlug(
  pluginName: string,
  mode: 'source' | 'sink' | 'transform' = 'source',
): string {
  const clean = (pluginName || '').toLowerCase().trim().replace(/^connector-/, '');

  // 1. Transform / 转换插件
  if (mode === 'transform' || clean.includes('transform')) {
    return 'sql';
  }

  // 2. CDC connectors / CDC 插件 (主要作为 source)
  const cdcMap: Record<string, string> = {
    'cdc-mysql': 'MySQL-CDC',
    'cdc-oracle': 'Oracle-CDC',
    'cdc-postgres': 'PostgreSQL-CDC',
    'cdc-sqlserver': 'SqlServer-CDC',
    'cdc-opengauss': 'Opengauss-CDC',
    'cdc-tidb': 'TiDB-CDC',
    'cdc-oceanbase': 'OceanBase',
    'cdc-mongodb': 'MongoDB-CDC',
  };
  if (cdcMap[clean]) {
    return cdcMap[clean];
  }

  // 3. Test & Virtual connectors / 模拟与测试连接器
  if (clean === 'fake') {
    return mode === 'source' ? 'FakeSource' : 'Fake';
  }
  if (clean === 'assert') {
    return 'Assert';
  }
  if (clean === 'console') {
    return 'Console';
  }

  // 4. File-based connectors / 文件类连接器
  const fileMap: Record<string, string> = {
    'file-local': 'LocalFile',
    localfile: 'LocalFile',
    'file-s3': 'S3File',
    s3file: 'S3File',
    'file-hdfs': 'HdfsFile',
    hdfsfile: 'HdfsFile',
    'file-ftp': 'FtpFile',
    ftpfile: 'FtpFile',
    'file-sftp': 'SftpFile',
    sftpfile: 'SftpFile',
    'file-oss': 'OssFile',
    ossfile: 'OssFile',
    'file-obs': 'ObsFile',
    obsfile: 'ObsFile',
    'file-cos': 'CosFile',
    cosfile: 'CosFile',
  };
  if (fileMap[clean]) {
    return fileMap[clean];
  }

  // 5. Special database casing in SeaTunnel docs / SeaTunnel 官方文档特定大小写
  if (clean === 'postgres' || clean === 'postgresql') {
    return mode === 'sink' ? 'PostgreSql' : 'PostgreSQL';
  }
  if (clean === 'mysql') {
    return 'Mysql';
  }
  if (clean === 'sqlserver') {
    return 'SqlServer';
  }
  if (clean === 'amazondynamodb') {
    return 'AmazonDynamoDB';
  }
  if (clean === 'amazonsqs') {
    return 'AmazonSqs';
  }

  // 6. Direct mapping from SPECIAL_IDENTIFIERS / 特殊标识映射
  if (SPECIAL_IDENTIFIERS[clean]) {
    return SPECIAL_IDENTIFIERS[clean];
  }

  // 7. Capitalize first letter as fallback / 默认首字母大写兜底
  return clean.charAt(0).toUpperCase() + clean.slice(1);
}

/**
 * Build official documentation direct URLs
 * 构建官方文档精准直达链接（区分 Source 与 Sink，支持中英文联动与官方多语言路由）
 *
 * @param pluginName - Plugin name / 插件名
 * @param version - SeaTunnel version / SeaTunnel 版本
 * @param locale - Current UI locale / 当前语言环境
 * @param category - Connector category / 连接器类别
 */
export function generateConnectorDocUrls(
  pluginName: string,
  version?: string,
  locale?: string,
  category?: string,
): {
  sourceDocUrl?: string;
  sinkDocUrl?: string;
} {
  const isZh = locale?.toLowerCase().startsWith('zh');
  // Official SeaTunnel website uses /zh-CN/docs for Chinese, /docs for English (default locale)
  // 官方 SeaTunnel 网站中文使用 /zh-CN/docs 前缀，英文默认语言使用 /docs 前缀
  const localePrefix = isZh ? '/zh-CN' : '';
  const capability = getConnectorCapability(pluginName, category);

  // Check if transform / 检查是否为转换插件
  if (category === 'transform' || pluginName.toLowerCase().includes('transform')) {
    const transformSlug = getConnectorDocSlug(pluginName, 'transform');
    const transformUrl = `https://seatunnel.apache.org${localePrefix}/docs/transforms/${transformSlug}/`;
    return {
      sourceDocUrl: transformUrl,
      sinkDocUrl: transformUrl,
    };
  }

  const sourceSlug = getConnectorDocSlug(pluginName, 'source');
  const sinkSlug = getConnectorDocSlug(pluginName, 'sink');

  // Authoritative canonical URLs on seatunnel.apache.org
  // 官方权威文档直达路径
  const sourceDocUrl = capability.supportsSource
    ? `https://seatunnel.apache.org${localePrefix}/docs/connectors/source/${sourceSlug}/`
    : undefined;

  const sinkDocUrl = capability.supportsSink
    ? `https://seatunnel.apache.org${localePrefix}/docs/connectors/sink/${sinkSlug}/`
    : undefined;

  return {sourceDocUrl, sinkDocUrl};
}

/**
 * Get semantic icon and theme color for a connector
 * 为连接器解析语义化图标与主题色（优先返回官方图标资源路径）
 *
 * @param pluginName - Connector name / 连接器名称
 * @param category - Category / 分类
 */
export function getConnectorSemanticIcon(
  pluginName: string,
  category?: string,
): {
  icon: LucideIcon;
  iconUrl?: string;
  bgColor: string;
  textColor: string;
  tagLabel?: string;
} {
  const name = (pluginName || '').toLowerCase().trim();
  const officialIcon = getConnectorOfficialIconUrl(pluginName);

  // Transform connectors / 转换类连接器
  if (category === 'transform' || name.includes('transform')) {
    return {
      icon: Shuffle,
      iconUrl: officialIcon || undefined,
      bgColor: 'bg-purple-100 dark:bg-purple-950/60',
      textColor: 'text-purple-700 dark:text-purple-300',
      tagLabel: 'Transform',
    };
  }

  // Streaming & Message Queues / 流计算与消息队列
  if (
    name === 'kafka' ||
    name === 'pulsar' ||
    name === 'rocketmq' ||
    name === 'rabbitmq' ||
    name === 'activemq' ||
    name.includes('sqs')
  ) {
    return {
      icon: Radio,
      iconUrl: officialIcon || undefined,
      bgColor: 'bg-amber-100 dark:bg-amber-950/60',
      textColor: 'text-amber-700 dark:text-amber-300',
      tagLabel: 'Messaging',
    };
  }

  // Modern OLAP & Data Warehouses / 现代分析型数仓
  if (
    name === 'clickhouse' ||
    name === 'doris' ||
    name === 'starrocks' ||
    name === 'iceberg' ||
    name === 'hudi' ||
    name === 'hive'
  ) {
    return {
      icon: Layers,
      iconUrl: officialIcon || undefined,
      bgColor: 'bg-emerald-100 dark:bg-emerald-950/60',
      textColor: 'text-emerald-700 dark:text-emerald-300',
      tagLabel: 'Warehouse',
    };
  }

  // File & Object Storage / 文件与对象存储
  if (
    name.startsWith('file-') ||
    name.includes('s3') ||
    name.includes('oss') ||
    name.includes('hdfs') ||
    name.includes('ftp')
  ) {
    return {
      icon: HardDrive,
      iconUrl: officialIcon || undefined,
      bgColor: 'bg-sky-100 dark:bg-sky-950/60',
      textColor: 'text-sky-700 dark:text-sky-300',
      tagLabel: 'Storage',
    };
  }

  // Search, NoSQL & Cache / 搜索、文档与缓存
  if (
    name === 'elasticsearch' ||
    name === 'easysearch' ||
    name === 'redis' ||
    name === 'mongodb' ||
    name === 'cassandra' ||
    name === 'neo4j'
  ) {
    return {
      icon: Search,
      iconUrl: officialIcon || undefined,
      bgColor: 'bg-indigo-100 dark:bg-indigo-950/60',
      textColor: 'text-indigo-700 dark:text-indigo-300',
      tagLabel: 'NoSQL/Search',
    };
  }

  // CDC Realtime capture / 实时 CDC
  if (name.startsWith('cdc-')) {
    return {
      icon: Activity,
      iconUrl: officialIcon || undefined,
      bgColor: 'bg-rose-100 dark:bg-rose-950/60',
      textColor: 'text-rose-700 dark:text-rose-300',
      tagLabel: 'CDC Stream',
    };
  }

  // Relational Databases / 关系型数据库与通用 JDBC
  if (
    name === 'jdbc' ||
    name.includes('mysql') ||
    name.includes('postgres') ||
    name.includes('oracle')
  ) {
    return {
      icon: DatabaseZap,
      iconUrl: officialIcon || undefined,
      bgColor: 'bg-blue-100 dark:bg-blue-950/60',
      textColor: 'text-blue-700 dark:text-blue-300',
      tagLabel: 'RDBMS',
    };
  }

  // General Connector / 通用连接器
  return {
    icon: Cable,
    iconUrl: officialIcon || undefined,
    bgColor: 'bg-slate-100 dark:bg-slate-800',
    textColor: 'text-slate-700 dark:text-slate-300',
    tagLabel: 'Connector',
  };
}
