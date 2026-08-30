# 更新说明 (Changelog)

所有重要变更都记录在此文件。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/),
版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### ✨ 新增

- **登录页背景图**:与 Docker_Manager_Go 同一张内置壁纸(`web/public/bg.jpg`),1Panel 风格
  背景图 + 深色遮罩,暗色/亮色主题下均适配;图片加载失败自动回退纯色背景
- **登录页右上角工具栏**:主题切换(明/暗,圆形扩散过渡)+ 语言切换(14 语言下拉),
  与 Docker_Manager_Go 登录页交互一致

### 🎨 改进

- 管理面板 UI/UX 重构:shadcn-vue 组件 + Design Tokens + 14 语言 i18n(zh-CN/zh-TW/en/
  ja/ko/ru/vi/es/id/uk/tr/pt-BR/ar/fa,含 RTL 支持)
- 移除误提交的本地验收截图(`web/shots/` 加入 .gitignore)

### 🚀 架构

- **License V3 架构升级**(详见 README「在线授权闭环」):
  - 激活凭据改为 `activation_token`(32B hex,数据库只存 SHA-256,明文仅激活响应返回一次)
  - 重放防护:timestamp ±5min + nonce 一次性(SHA-256 存储,1h 清理)
  - 版本控制:`minimum_client_version` 提示升级 / `blocked_versions` 封禁
  - 安全事件:invalid_signature / invalid_token / replay_detected / rate_limit_exceeded /
    tampered_timestamp / device_limit_exceeded / client_version_blocked
  - 新表:customers / subscriptions / activation_tokens / security_events / security_nonces /
    server_settings
  - 新管理端点:/customers、/subscriptions、/security-events、/settings(GET/PUT)
- **客户与订阅管理**:签发 API 透传 `customer_id` / `subscription_id`(V2.1 契约字段)
- **仓库迁移到 DockerManger 组织**:模块路径 / 安装脚本 / GHCR 镜像 / 文档链接全部更新

### 🐛 修复

- **许可证列表状态筛选 42P18**:COUNT 查询曾复用主查询的 `$3` 参数编号(仅传 1 个参数),
  导致 PG 无法绑定参数报 `could not determine data type of parameter $1`;现独立编号,
  并支持「已过期」动态筛选(按 `expires_at` 计算,数据库 status 列不维护过期状态)
- **安全事件类型筛选 42P18**:SecurityEventRepo.List 同样存在 COUNT 查询参数编号
  错位(事件类型筛选全部报 42P18),已独立编号修复(含回归测试,7 种事件类型全覆盖)
- **主界面重复标题**:顶栏标题与页面内 PageHeader 重复显示两遍,已移除顶栏
  title/description(页面内保留一份)
- 签发 API 透传 customer_id/subscription_id(此前被丢弃,payload 缺关联字段)
- licenses 外键列显式 `::uuid` 转换(防 PG 推断 text 报 42804)
- 订阅创建传客户数据库 UUID(customer_id 列是 UUID 外键,非展示 ID)
- resolveLicense 非 UUID 格式未知 ID 直接返回 404(防 PG uuid 列语法错误 500)
- 管理 API 的 `:id` 兼容数据库 UUID 与展示 ID(DMG-...)
- 测试清理表顺序遵循外键(先删引用者再删被引用者,防 FK 约束失败)

## [v1.0.1] - 2026-08-30

### 🐛 修复

- 管理 API 的 `:id` 兼容数据库 UUID 与展示 ID(DMG-...)(列表返回 id 直接可查,防 404 坑)
- resolveLicense 非 UUID 格式未知 ID 直接返回 404(防 PG uuid 列语法错误 500)

### ⚙️ CI

- binaries job 补 `web/dist` 占位(gofmt 前 go:embed 需要 dist 目录存在)

## [v1.0.0] - 2026-08-29

### ✨ 新增

- 单容器全包镜像:内置 license-server + nginx 反代 + PostgreSQL,对外仅暴露 80 端口,
  compose 一条命令部署,零配置开箱即用
- `install.sh` 一键部署:检测环境 → 创建部署目录 → 下载 compose → 拉镜像 → 启动 →
  打印管理员账号/密码/公钥;支持国内镜像加速与强制重装
- 在线授权闭环:activate / verify / deactivate API + 设备绑定与上限(事务行锁) +
  signing_keys 密钥注册表 + 设备管理后台 + IP 限流
- 管理后台(Vue3 + TS):License 签发/延期/吊销/导出、审计日志、密钥管理
- 域名接入:Cloudflare A 记录 Proxied + 内置 nginx 反代,免手动 nginx 配置

### 🔒 安全

- Ed25519 非对称签名(私钥 0600,永不进代码/镜像;CI gitleaks 扫描)
- argon2id 密码哈希 + JWT(token_version)+ TOTP 2FA + 登录限流
- Docker 安全加固:read_only rootfs + cap_drop ALL + no-new-privileges

### 🐛 修复

- Alpine 3.20 postgresql16 二进制路径为 `/usr/libexec/postgresql16`(非 /usr/lib/postgresql/16/bin)
- 启动 PG 前创建 /run/postgresql 目录(socket 锁文件目录,Alpine 容器默认不存在)
- 安装命令与 compose 下载改走 github.com raw 端点,规避 raw CDN 滞后

---

[Unreleased]: https://github.com/DockerManger/Docker_Manager_License/compare/v1.0.1...master
[v1.0.1]: https://github.com/DockerManger/Docker_Manager_License/compare/v1.0.0...v1.0.1
[v1.0.0]: https://github.com/DockerManger/Docker_Manager_License/releases/tag/v1.0.0
