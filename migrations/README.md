# migrations

版本化数据库迁移的 **真实位置**: `internal/database/migrations/*.sql`

说明:Go `go:embed` 只能嵌入包目录内的文件,为保证单二进制部署
(Docker 镜像无需额外拷贝 SQL),迁移文件放在 `internal/database/migrations/`,
由 `internal/database` 包在启动时按文件名序号自动执行,已执行版本记录在
`schema_migrations` 表。

- 所有 Schema 变更必须新增版本化 SQL 文件(如 `002_xxx.sql`),禁止运行时自动猜结构 ALTER。
- 文件名前缀为执行顺序序号,已发布的文件内容**不可修改**(修改 = 新增文件)。
