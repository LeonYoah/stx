# 精选模板草稿 v2（供审查）

## 版本注意：`plugin_input` / `plugin_output`

| 字段 | 引入版本 | 低版本等价 |
|---|---|---|
| `plugin_output` | **SeaTunnel ≥ 2.3.9**（#8072） | `result_table_name` |
| `plugin_input` | **SeaTunnel ≥ 2.3.9** | `source_table_name` |

本草稿种子默认写 `plugin_*`（面向 ≥2.3.9）。实现插入精选时须按目标集群版本做改写（仓库已有同类逻辑：`internal/apps/sync/service.go` 对 `< 2.3.9` 把 `plugin_input`→`source_table_name`、`plugin_output`→`result_table_name`）。**禁止**对低版本集群原样插入 `plugin_*`。

1. **按块拆分、自由组合**：内置以 `env` / `source` / `transform` / `sink` 片段为主。
2. **一键组合模板**：另存时扫描并纳入上述 **四节**（有则必带）。
3. **变量策略**：仅密码/密钥类凭证使用 `{{...}}`；其余写死示例值。
4. **Hive**：基础 / Kerberos / S3。**文件 Source**：Local / S3 / Hdfs / Ftp。

## 目录

```
curated-draft/
├── env/         # 运行环境
├── source/      # 源端片段
├── transform/   # 转换片段
└── sink/        # 目标端片段
```

### env

| 文件 | 说明 |
|---|---|
| `batch.conf` | 批处理 |
| `streaming.conf` | 流处理 + checkpoint |

### source

| 文件 | 说明 |
|---|---|
| `fake.conf` | FakeSource 冒烟 |
| `jdbc-mysql-single.conf` | JDBC 单表（`split.size` / `fetch_size` + 切分注释） |
| `jdbc-mysql-multi.conf` | JDBC 多表（同上） |
| `mysql-cdc-single.conf` | MySQL-CDC 单表 |
| `mysql-cdc-multi.conf` | MySQL-CDC 多表 |
| `kafka-json.conf` | Kafka JSON |
| `localfile-json.conf` | 本地 JSON |
| `s3file-json.conf` | S3 JSON（密钥走变量） |
| `hdfsfile-json.conf` | HDFS JSON |
| `ftpfile-json.conf` | FTP JSON（密码走变量） |

### transform

| 文件 | 说明 |
|---|---|
| `copy-field.conf` | Copy 字段拷贝 |

### sink

| 文件 | 说明 |
|---|---|
| `console.conf` | Console |
| `jdbc-mysql.conf` | JDBC MySQL |
| `jdbc-mysql-multi-route.conf` | JDBC 多表路由 `${table_name}` |
| `kafka-canal-json.conf` | Kafka canal_json |
| `hive-basic.conf` | Hive 基础 |
| `hive-kerberos.conf` | Hive Kerberos |
| `hive-s3.conf` | Hive + S3 warehouse |

## 组合示例（非种子，仅说明）

| 场景 | 建议组合 |
|---|---|
| 冒烟 | env/batch + source/fake + sink/console |
| JDBC 单表 | env/batch + source/jdbc-mysql-single + sink/jdbc-mysql |
| JDBC 多表 | env/batch + source/jdbc-mysql-multi + sink/jdbc-mysql-multi-route |
| CDC→MySQL | env/streaming + source/mysql-cdc-single + sink/jdbc-mysql |
| CDC→Kafka | env/streaming + source/mysql-cdc-multi + sink/kafka-canal-json |
| Kafka→MySQL | env/streaming + source/kafka-json + sink/jdbc-mysql |
| 文件→MySQL | env/batch + source/{local\|s3\|hdfs\|ftp}file-json + sink/jdbc-mysql |
| JDBC→Hive | env/batch + source/jdbc-mysql-single + sink/hive-{basic\|kerberos\|s3} |
| JDBC→Console | env/batch + source/jdbc-mysql-single + sink/console |

## 凭证变量一览（仅这些用 `{{}}`）

- `{{jdbc_password}}` / `{{cdc_password}}` / `{{ftp_password}}`
- `{{s3_access_key}}` / `{{s3_secret_key}}`
- Kerberos：路径类示例写死；若你希望 keytab 路径也变量化可再说

## 待确认（已关闭）

- 精选 v2 块级草稿已审定。
- 一键组合须覆盖 env / source / transform / sink。
- 一期以分块种子为主；整作业「菜谱」可选后续再加。
