# 部署流程(一步一步照着做)

> 目标:把 License Server(签发/管理端)部署到你的服务器上,并让 Docker_Manager_Go 面板成功激活许可证。
> 全程只需复制粘贴命令,不需要懂 Linux。以下命令中的 `<你的IP>` 换成你的服务器公网 IP。

---

## 第 0 步:准备

1. 一台有公网 IP 的 Linux 服务器(已装 Docker 和 Compose 插件)。
   检查:在服务器上执行 `docker --version && docker compose version`,有输出即可。
2. 一个域名(方式二才需要,方式一不需要)。

---

## 方式一:用 IP 直连(最简单,先跑通再升级)

### 1. 部署 License Server(一条命令)

SSH 登录服务器后,执行:

```bash
curl -fsSL https://github.com/DockOrae/DockOrae-Auth/raw/refs/heads/master/deploy/install.sh | bash
```

等它打印出「✓ 部署完成」之类的字样。它会自动:

- 在 `~/dockorae-auth` 建好目录和 compose 文件
- 拉镜像并启动(对外只开 **80 端口**,内置 nginx + PostgreSQL)
- 首次启动自动生成管理员密码、JWT secret、Ed25519 私钥

> 重复执行脚本会检测到已部署,显示状态后退出,不会重复初始化。

### 2. 查看管理员密码(只打印一次)

```bash
cd ~/dockorae-auth && docker compose logs license-server | grep -E "用户名|密码"
```

记下「密码:」后面的内容(例如 `J93gKMMCsb77RNym`)。这是管理后台的登录密码。

### 3.(重要)统一签名私钥,让面板能验签

License Server 首次启动会自动生成一把随机私钥。**为了让 Docker_Manager_Go 内置的公钥能验签,必须使用与它配对的私钥**(即仓库里 `private/license.key`)。

在**你的电脑**(不是服务器)上,把仓库的私钥上传到服务器:

```bash
scp /d/text/DockOrae-Auth/private/license.key root@<你的IP>:/root/dockorae-auth/private/license.key.new
```

然后在服务器上替换并重启:

```bash
cd /root/dockorae-auth/private
cp license.key license.key.bak                # 备份自动生成的旧钥
mv license.key.new license.key
chmod 600 license.key
cd .. && docker compose restart license-server
```

验证公钥是否已同步(重启后注册表自动更新,出现 `dD7pzvzY...` 开头即成功):

```bash
docker compose logs license-server | grep -A1 "PUBLIC KEY"
```

> 跳过这步的后果:面板激活时会报验签失败 / 许可证不可用。
> 如果私钥文件还没在电脑上,先 `cd /d/text/DockOrae-Auth && git pull`,私钥在 `private/` 目录(已被 .gitignore 排除,不会传到 GitHub)。

### 4. 访问管理后台

浏览器打开:

```
http://<你的IP>
```

用第 2 步的账号(`admin`)和密码登录。**登录后第一件事:在「设置」里改掉初始密码**(强烈建议再开 TOTP 两步验证)。

### 5. 签发一张许可证

管理后台 →「许可证」→「＋ 签发 License」:

| 字段 | 填什么 |
|---|---|
| 客户 | 随便,例如 `kejizero` |
| 套餐 | `pro` |
| 功能 | 勾选 `compose`、`container_create`、`appstore`(缺哪个,面板就禁用哪个功能) |
| 有效期 | 例如 365 天 |
| 最大设备数 | 例如 3(超过这个数量的面板激活会被拒) |

点签发,复制生成的 **License Key**(很长的一串,以 `eyJ...` 开头)。

### 6. 让 Docker_Manager_Go 面板连上 License Server

在**面板的部署目录**(例如 `/opt/docker-manager`),给 compose 文件加一行环境变量:

```bash
cd /opt/docker-manager
sed -i 's|      - PORT=8080|      - PORT=8080\n      - DM_LICENSE_SERVER_URL=http://<你的IP>/license-api|' docker-compose.yml
docker compose up -d --force-recreate
```

> 如果面板和 License Server 在同一台服务器,`<你的IP>` 就是服务器自己的公网 IP。

### 7. 面板里激活

浏览器打开面板 →「面板设置」→「授权」→「添加」→ 粘贴第 5 步的 License Key → 激活。

看到「**在线验证通过**」、许可证信息显示出来,就大功告成了。

---

## 方式二:用域名 + HTTPS(推荐长期使用)

和方式一的区别:域名经过 Cloudflare,浏览器和面板走 `https://域名/license-api` 访问 License Server,不用记 IP。

### 1. 部署 License Server

同方式一第 1~3 步(部署 + 取密码 + 统一私钥)。

### 2. Cloudflare 加 DNS 记录

假设域名是 `license.example.com`(换成你自己的):

1. 登录 Cloudflare → 选择你的域名 → **DNS** → **添加记录**:
   - 类型:`A`
   - 名称:`license`(或任意子域名)
   - IPv4 地址:`<你的IP>`
   - **代理状态:开(橙色云朵)** ← 关键,这样才有免费 HTTPS
   - TTL:自动
2. **SSL/TLS → 加密模式 → 选 `Flexible`** ← 关键
   - 因为 License Server 只监听 80 端口(HTTP),由 Cloudflare 负责加密,回源走 HTTP。
   - 注意:加密模式是全站生效的,如果你还有其他用「Full」的代理站点,改成 Flexible 后它们也能正常访问(CF 会跟随跳转),只是回源变成 HTTP。

### 3. 验证域名通了

```bash
curl -s https://license.example.com/license-api/healthz
```

返回 `{"status":"ok"}` 就成功了。

### 4. 管理后台

浏览器打开 `https://license.example.com/`,用管理员账号登录(同方式一第 2 步的密码)。

### 5. 签发许可证

同方式一第 5 步。

### 6. 面板连授权服务器(两种选一)

**方法 A(推荐,零配置)**:Docker_Manager_Go 源码内置的默认地址就是官方域名;只要你的 License Server 部署在官方地址,面板什么都不用配。

**方法 B(自建/换域名)**:在面板 compose 里加环境变量:

```bash
cd /opt/docker-manager
sed -i 's|      - PORT=8080|      - PORT=8080\n      - DM_LICENSE_SERVER_URL=https://license.example.com/license-api|' docker-compose.yml
docker compose up -d --force-recreate
```

### 7. 面板里激活

同方式一第 7 步。

---

## 日常运维

| 想做什么 | 命令 |
|---|---|
| 看服务状态 | `cd ~/dockorae-auth && docker compose ps` |
| 看日志(含公钥) | `docker compose logs -f license-server` |
| 重启服务 | `docker compose restart license-server` |
| 更新到新版 | `docker compose pull && docker compose up -d` |
| 重新部署(清空重来) | `DML_FORCE=1 bash ~/dockorae-auth/deploy-install.sh`(或先 `docker compose down` 再跑安装脚本) |

## 常见问题

| 现象 | 原因 | 解决 |
|---|---|---|
| 面板激活报 `license.serverUnreachable` | 面板连不上授权服务器 | 检查 `DM_LICENSE_SERVER_URL` 是否填对;在面板服务器上 `curl http://<你的IP>/license-api/healthz` 看通不通 |
| 面板激活报验签失败/许可证无效 | 私钥和公钥不配对 | 按方式一第 3 步统一私钥后重启 License Server |
| 后台登录不上 | 密码记错了 | `docker compose logs license-server \| grep 密码` 重新查看 |
| 第 3 台设备激活被拒(409) | 设备数达到上限 | 后台「许可证 → 详情 → 设备管理」解绑不再用的设备,或签发更大设备数的证 |
| 吊销后面板还在用 | 吊销最长 24h 后生效 | 在面板「授权」页点「立即验证」即时生效 |
| `https://域名/license-api/healthz` 打不开 | DNS 代理没开 / SSL 模式不对 | 检查橙色云朵是否开启、SSL 模式是否为 Flexible、A 记录 IP 是否填对 |

---

## 验证命令速查

```bash
# License Server 健康检查
curl -s http://<你的IP>/license-api/healthz          # 方式一
curl -s https://license.example.com/license-api/healthz  # 方式二

# 面板侧看许可证状态
# 面板 → 设置 → 授权 → 显示「在线验证通过」
```
