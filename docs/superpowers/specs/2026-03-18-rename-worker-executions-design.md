# 将表 worker_executions 重命名为 executions

**日期**: 2026-03-18
**状态**: 已批准

## 背景

数据库表 `worker_executions` 名称冗余，`worker_` 前缀在上下文中已无必要。将其重命名为 `executions` 使命名更简洁统一。

## 范围

- `internal/store/db.go`：新增迁移
- `internal/store/execution_store.go`：更新 SQL 引用

## 数据库迁移

在 `db.go` 的 `migrations` 切片中追加以下 5 条迁移（版本 16–20）：

| 版本 | 名称 | SQL |
|------|------|-----|
| 16 | `20260318_rename_table_worker_executions_to_executions` | `ALTER TABLE worker_executions RENAME TO executions` |
| 17 | `20260318_drop_index_worker_executions_worker_id` | `DROP INDEX idx_worker_executions_worker_id` |
| 18 | `20260318_create_index_executions_worker_id` | `CREATE INDEX IF NOT EXISTS idx_executions_worker_id ON executions(worker_id)` |
| 19 | `20260318_drop_index_worker_executions_session_id` | `DROP INDEX idx_worker_executions_session_id` |
| 20 | `20260318_create_index_executions_session_id` | `CREATE INDEX IF NOT EXISTS idx_executions_session_id ON executions(session_id)` |

迁移按顺序执行，每条各一个事务，符合现有 `applyMigrations` 实现。

## SQL 引用更新

`execution_store.go` 中共 6 处 `worker_executions` 改为 `executions`：

1. `execSelect` 常量：`FROM worker_executions e` → `FROM executions e`
2. `Create` 方法：`INSERT INTO worker_executions` → `INSERT INTO executions`
3. `UpdateStatus`：`UPDATE worker_executions` → `UPDATE executions`
4. `UpdateLogs`：`UPDATE worker_executions` → `UPDATE executions`
5. `UpdateResult`：`UPDATE worker_executions` → `UPDATE executions`
6. `UpdatePID`：`UPDATE worker_executions` → `UPDATE executions`

代码逻辑无任何变化，仅替换表名字符串。

## 不在范围内

- 不修改 Go 结构体、类型名称（`WorkerExecution`、`ExecutionStore` 等）
- 不修改 API 路由或前端代码
- 不修改其他测试以外的文件

## 测试

现有的 `internal/store/db_test.go` 和 `internal/store/execution_store_test.go` 应在迁移后继续通过，无需额外修改。
