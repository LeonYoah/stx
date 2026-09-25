# 数据库规范

> 本项目的数据库使用模式与约定。

---

## 概述

项目使用 **GORM** 做关系型持久化。支持的数据库为 **SQLite**（默认）、**MySQL** 和 **PostgreSQL**。数据库在 `internal/db/database.go` 中初始化；迁移通过 `internal/db/migrator/migrator.go` 使用 GORM 的 `AutoMigrate` 执行。Repository 接收 `*gorm.DB`，所有查询必须使用 `WithContext(ctx)`。多步写操作使用事务；部分 repo 提供 `Transaction(ctx, fn)`，在回调中传入基于事务的 repository。

---

## 查询模式

- **Context**：每次查询都使用 `r.db.WithContext(ctx)`（或事务内的 `tx`），以便超时与取消能够传递。
- **Preload**：handler 需要关联数据时使用 `Preload("Nodes")`（或其他关联名）；repository 方法可提供选项（如 `GetByID(ctx, id, preloadNodes bool)`）。
- **筛选与分页**：列表方法接收筛选结构体（如带 `Name`、`Status`、`Page`、`PageSize` 的 `ClusterFilter`）。先加 `Where` 条件，再 `Count`，再 `Offset/Limit` 和 `Order`，最后 `Find`。
- **哨兵错误**：「未找到」时，repository 将 `gorm.ErrRecordNotFound` 映射为包内错误（如 `ErrClusterNotFound`）并返回；调用方用 `errors.Is(err, ErrClusterNotFound)` 判断。
- **批量/多行**：使用 `Find(&list)`、`Updates(map)` 或 `Where(...).Delete()`；通过 Preload 或批量操作避免 N+1。

示例（来自 `cluster/repository.go`）：

```go
func (r *Repository) GetByID(ctx context.Context, id uint, preloadNodes bool) (*Cluster, error) {
	var cluster Cluster
	query := r.db.WithContext(ctx)
	if preloadNodes {
		query = query.Preload("Nodes")
	}
	if err := query.First(&cluster, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrClusterNotFound
		}
		return nil, err
	}
	return &cluster, nil
}
```

---

## 迁移

- **工具**：GORM `AutoMigrate`，位于 `internal/db/migrator/migrator.go`。无独立迁移文件；所有 model 在一个 `AutoMigrate(...)` 调用中注册。
- **需要迁移的 Model**：新 GORM model 加入 `migrator.go` 的 `AutoMigrate` 列表（如 `&monitor.ProcessEvent{}`）。表名遵循 GORM 默认（结构体名的 snake_case 复数，除非重写）。
- **迁移时禁用外键**：全局 GORM 配置使用 `DisableForeignKeyConstraintWhenMigrating: true`。
- **仅 MySQL**：存储过程（如仪表盘用）放在 `support-files/sql/`，仅在 `db.GetDatabaseType() == "mysql"` 时执行。

---

## 命名约定

- **表名**：GORM 默认 — 结构体名 snake_case 复数（如 `Cluster` → `clusters`，`ClusterNode` → `cluster_nodes`）。需要时用 `TableName()` 重写。
- **列名**：由结构体字段名得到 snake_case（如 `ClusterID`、`HostID` → `cluster_id`、`host_id`）。使用 `gorm` 结构体标签指定列名与索引。
- **索引**：通过结构体标签定义，如 `gorm:"index"` 或 `gorm:"uniqueIndex:idx_name"`。业务唯一约束（如集群名）除唯一索引外，在 repository 逻辑中显式检查（插入/更新前检查）。

---

## 事务

- **使用场景**：需要多步写操作要么全成功要么全失败时（如创建集群 + 节点，或更新模板 + 同步节点配置）。
- **写法**：使用 `r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { ... })`，或使用 repository 提供的事务封装：

```go
// 来自 config/repository.go
func (r *Repository) Transaction(ctx context.Context, fn func(tx *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}
```

- **在 Service 中的用法**：调用 `repo.Transaction(ctx, func(tx *Repository) error { ... })`，在回调内用 `tx` 做所有读写，保证同一事务。

---

## 常见错误

- 查询时忘记 `WithContext(ctx)`，导致追踪与请求取消无法传递。
- 向 handler 直接返回原始 `gorm.ErrRecordNotFound`；应映射为领域错误（如 `ErrClusterNotFound`）以便 handler 映射为 HTTP 404。
- 需要一致性时在循环中多次更新却未包在事务中。
- 依赖外键级联删除而未考虑项目配置（`DisableForeignKeyConstraintWhenMigrating`）；建议在事务中显式删除（如先删节点再删集群），参见 `cluster/repository.Delete`。
- **多数据库方言不兼容**：
  - 严禁在 GORM tag 中使用 MySQL 独有类型 `gorm:"type:longtext"` 或 `mediumtext`。
  - **长脚本 / 大配置字段例外**：模型字段必须使用 `db.ScriptText`（方言感知类型：MySQL → `MEDIUMTEXT`，PostgreSQL / SQLite → `TEXT`），不要再写 `gorm:"type:text"`（会覆盖方言分流），也不允许裸写 `type:longtext` / `type:mediumtext`。
  - **普通短文本**：仍使用 `gorm:"type:text"`（或省略 type，交给 GORM 默认）。
  - API / DTO 边界保持 `string`：写入模型时用 `db.ScriptText(s)`，读出到 string 时用 `.String()` 或 `string(x)`。
  - 严禁裸写 `Where("col LIKE ?", ...)`（PostgreSQL 默认大小写敏感，会导致查询遗漏），统一使用 `Where("LOWER(col) LIKE LOWER(?)", ...)`。
  - 严禁在原生 SQL 字符串中夹带 MySQL 反引号 \`（PostgreSQL 会直接报语法错误）。
  - 本地或提交前必须运行 `./scripts/test_db_compat.sh` 确保三库迁移与一致性测试通过。

## 场景：创建记录时保留显式 `false`

### 1. 范围

- API 创建请求中的布尔字段既允许省略，也允许显式传入 `false`。
- 数据模型原本使用 `gorm:"default:true"`，但业务必须区分“未传”和“明确关闭”。

### 2. 签名

```go
type CreateRequest struct {
    Enabled *bool `json:"enabled,omitempty"`
}

type Model struct {
    Enabled bool `gorm:"not null"`
}
```

### 3. 规则

- 请求结构使用 `*bool` 区分省略与显式 `false`。
- Service 在请求省略时设置业务默认值。
- Model 不使用会覆盖 Go 零值的 `default:true` 标签，创建 SQL 必须显式写入最终布尔值。

### 4. 校验与错误对应表

| 输入 | 保存值 |
| --- | --- |
| 未传 `enabled` | 使用 Service 定义的默认值 |
| `enabled=true` | `true` |
| `enabled=false` | `false` |

### 5. Good / Base / Bad

- Good：请求传 `false`，数据库和响应都保持 `false`。
- Base：请求未传值，Service 使用清晰的默认值。
- Bad：普通 `bool` 配合 `gorm:"default:true"`，导致显式 `false` 被 GORM 改成 `true`。

### 6. 必须有的测试

- Handler 或 Service 测试必须覆盖省略、`true`、`false` 三种输入。
- 至少一条数据库测试确认创建后重新读取仍为 `false`。

### 7. 错误与正确示例

错误：

```go
Enabled bool `json:"enabled" gorm:"default:true"`
```

正确：

```go
enabled := true
if req.Enabled != nil {
    enabled = *req.Enabled
}
model.Enabled = enabled
```
