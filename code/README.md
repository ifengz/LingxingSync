# 领星同步机

领星 OpenAPI 数据同步工具：Go 单二进制 + MySQL + 浏览器 UI，零构建步骤。

---

## 本地开发（Docker MySQL）

**最短路径：**

```bash
# 1. 启动 MySQL 容器（一次性）
docker run -d --name lingsync-mysql \
  -e MYSQL_ROOT_PASSWORD=rootpass \
  -e MYSQL_DATABASE=lingsync \
  -e MYSQL_USER=lingsync_rw \
  -e MYSQL_PASSWORD=devpass \
  -p 3306:3306 \
  mysql:8.0

# 2. 复制并填写配置
cp config.example.yaml config.yaml
# 编辑 config.yaml：
#   database.password 改为 devpass
#   accounts[].app_key / app_secret 填入真实凭证

# 3. 编译并运行（需要 Go 1.22+）
make build && ./lingxing-sync

# 4. 打开浏览器
open http://127.0.0.1:7799
```

> 数据库迁移由程序**启动时自动执行**（幂等 `CREATE TABLE IF NOT EXISTS`），无需手动操作。

### 停止容器

```bash
docker stop lingsync-mysql     # 暂停
docker start lingsync-mysql    # 恢复
docker rm -f lingsync-mysql    # 彻底删除
```

---

## 编译

```bash
make build
# 等价：go build -ldflags="-s -w" -o lingxing-sync .
# 产物：./lingxing-sync（约 15 MB 单二进制，零运行时依赖）
```

## 用已有 raw 回刷历史日维

当 `ls_*` 原始表已有数据、但 `listing_daily_metrics` 尚未投影时，使用一次性命令回刷指定日期范围：

```bash
./lingxing-sync \
  -rebuild-listing-daily \
  -rebuild-date-from 2026-01-01 \
  -rebuild-date-to 2026-08-29
```

可选 `-rebuild-account` 限定账号，`-rebuild-store` 限定店铺 SID。该模式只读取已有 `ls_*` raw 并写回 `listing_daily_metrics`，不调用领星、不创建同步任务、不执行 cron；不新增表、队列或锁。`operations-log-v3` 等读取 `listing_daily_metrics` 的数据集会随之读到回刷后的值。

---

## Makefile 命令一览

| 命令 | 作用 |
|---|---|
| `make tidy` | 拉取/更新依赖 |
| `make build` | 编译单二进制 |
| `make run` | 编译并前台运行 |
| `make fmt` | 格式化所有 Go 代码 |
| `make vet` | 静态检查 |
| `make clean` | 删除编译产物 |

---

## 宝塔云服务器部署

### 1. 克隆代码

```bash
cd /www/wwwroot
git clone https://github.com/ifengz/LingxingSync.git lingxing-sync
cd lingxing-sync/code          # 代码在 code/ 子目录
cp config.example.yaml config.yaml
nano config.yaml               # 填入 DB 密码 + 领星凭证
```

### 2. 安装 Go（如未安装）

在宝塔软件商店或 SSH 按 Go 官方安装方式安装 Go 1.23+，然后确认：

```bash
go version   # → go version go1.23.x linux/amd64
```

### 3. 编译

```bash
cd /www/wwwroot/lingxing-sync/code
make build
# 验证：ls -lh lingxing-sync
```

### 4. Supervisor 守护进程

宝塔 → 软件商店 → Supervisor → 添加守护进程（或直接写配置文件）：

```ini
[program:lingxing-sync]
command=/www/wwwroot/lingxing-sync/code/lingxing-sync
directory=/www/wwwroot/lingxing-sync/code
user=www
autostart=true
autorestart=true
startretries=3
stdout_logfile=/www/wwwroot/lingxing-sync/code/logs/stdout.log
stdout_logfile_maxbytes=50MB
stdout_logfile_backups=5
stderr_logfile=/www/wwwroot/lingxing-sync/code/logs/stderr.log
stderr_logfile_maxbytes=20MB
environment=HOME="/www/wwwroot/lingxing-sync/code"
```

```bash
mkdir -p /www/wwwroot/lingxing-sync/code/logs
supervisorctl reread && supervisorctl update
supervisorctl start lingxing-sync
supervisorctl status lingxing-sync   # 期望：RUNNING
```

### 5. Nginx 反代

宝塔 → 网站 → 添加站点 → 反向代理 → 目标 `http://127.0.0.1:7799`，或手动写配置：

```nginx
server {
    listen 80;
    server_name sync.yourdomain.com;

    client_max_body_size 60M;   # 对账文件上传

    location / {
        proxy_pass         http://127.0.0.1:7799;
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_read_timeout 300;
    }
}
```

> 端口 7799 **不对外开放**，只经 Nginx 代理访问。宝塔 → 安全 → 确保 7799 不在防火墙开放列表中。

### 6. 验证

```bash
supervisorctl status lingxing-sync     # RUNNING
ss -tlnp | grep 7799                   # LISTEN
curl http://127.0.0.1:7799/api/status  # {"ok":true,...}
```

---

## 升级流程

```bash
cd /www/wwwroot/lingxing-sync
git fetch origin
git checkout main
git pull --ff-only origin main
cd code
make build
supervisorctl restart lingxing-sync

# 程序启动时只执行 schema_migrations 中尚未记录的 migrations/*.sql
```

---

## 建数据库和用户（首次）

```sql
CREATE DATABASE lingsync CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'lingsync_rw'@'127.0.0.1' IDENTIFIED BY 'your_password';
GRANT ALL PRIVILEGES ON lingsync.* TO 'lingsync_rw'@'127.0.0.1';
-- 可选：只读账号供外部 BI 工具连接
CREATE USER 'lingsync_ro'@'%' IDENTIFIED BY 'readonly_password';
GRANT SELECT ON lingsync.* TO 'lingsync_ro'@'%';
FLUSH PRIVILEGES;
```

---

## 文档索引

| 文档 | 内容 |
|---|---|
| [doc/core/00-overview.md](../doc/core/00-overview.md) | 项目概述 |
| [doc/core/03-config.md](../doc/core/03-config.md) | 配置字段完整说明 |
| [doc/core/04-api.md](../doc/core/04-api.md) | HTTP API 规范 |
| [doc/core/06-deployment.md](../doc/core/06-deployment.md) | 宝塔部署详细指南 |
| [doc/core/07-add-endpoint.md](../doc/core/07-add-endpoint.md) | 如何零代码新增接口 |
| [doc/core/08-api-reference.md](../doc/core/08-api-reference.md) | 领星 OpenAPI 参考 |
