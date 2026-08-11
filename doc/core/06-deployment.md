# 领星同步机 — 宝塔部署指南（宪法层）

> 不用 Docker。Go 单二进制 + Supervisor 守护 + Nginx 反代。

---

## 1. 前置条件

| 组件 | 版本 | 宝塔路径 |
|---|---|---|
| 宝塔面板 | 7.x+ | — |
| MySQL | 8.0 | 软件商店 → MySQL 8.0 |
| Nginx | 1.24+ | 软件商店 → Nginx |
| Supervisor | 任意 | 软件商店 → Supervisor |
| Go | 1.23+ | 手动安装（见 §2）|
| 服务器 | 1C2G+ Linux | — |

---

## 2. 安装 Go（SSH 操作）

在宝塔软件商店或 SSH 按 Go 官方安装方式安装 Go 1.23+（检查最新版：https://go.dev/dl/），然后验证：

```bash
go version   # 应输出：go version go1.23.x linux/amd64
```

---

## 3. 建数据库和用户

**方式一：宝塔面板操作**
1. 宝塔 → 数据库 → 添加数据库
2. 数据库名：`lingsync`
3. 用户名：`lingsync_rw`，密码：自定义强密码
4. 权限：本地访问（127.0.0.1）

**方式二：SSH 操作**
```bash
mysql -u root -p
```
```sql
CREATE DATABASE lingsync CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'lingsync_rw'@'127.0.0.1' IDENTIFIED BY 'your_password';
GRANT ALL PRIVILEGES ON lingsync.* TO 'lingsync_rw'@'127.0.0.1';
FLUSH PRIVILEGES;
```

---

## 4. 部署代码

```bash
cd /www/wwwroot
git clone https://github.com/ifengz/LingxingSync.git lingxing-sync
cd lingxing-sync/code

# 复制配置模板
cp config.example.yaml config.yaml
# 编辑配置（填入 DB 密码、领星 app_key/secret）
nano config.yaml
```

---

## 5. 初始化数据库表

程序启动时会按文件名自动执行 `migrations/*.sql`，全部迁移使用幂等语句，首次启动和升级都不需要手动导入。确认数据库账号可连接后，直接执行编译和启动步骤即可。

---

## 6. 编译二进制

```bash
cd /www/wwwroot/lingxing-sync/code

make build

# 验证
./lingxing-sync -config config.yaml -h
```

编译产物：`./lingxing-sync`（单二进制，约 15MB）

---

## 7. Supervisor 配置（进程守护）

宝塔 → 软件商店 → Supervisor → 设置 → 添加守护进程：

```ini
[program:lingxing-sync]
command=/www/wwwroot/lingxing-sync/code/lingxing-sync -config /www/wwwroot/lingxing-sync/code/config.yaml
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
# 确保 logs 目录存在
mkdir -p /www/wwwroot/lingxing-sync/code/logs

# 重载 Supervisor
supervisorctl reread && supervisorctl update

# 启动服务
supervisorctl start lingxing-sync

# 查看状态
supervisorctl status lingxing-sync
```

---

## 8. Nginx 反代配置

**方式一：宝塔面板**
1. 宝塔 → 网站 → 添加站点
2. 域名：`sync.yourdomain.com`（或用 IP 直接访问跳过）
3. 站点创建后 → 设置 → 反向代理 → 添加：
   - 代理名称：lingxing-sync
   - 目标 URL：`http://127.0.0.1:7799`

**方式二：手动 Nginx 配置**（放入宝塔站点的 `nginx.conf`）

```nginx
server {
    listen 80;
    server_name sync.yourdomain.com;

    # 上传文件大小限制（对账文件可能较大）
    client_max_body_size 60M;

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

---

## 9. 验证部署

```bash
# 检查进程
supervisorctl status lingxing-sync
# 期望：lingxing-sync   RUNNING   pid 12345, uptime 0:00:30

# 检查端口
ss -tlnp | grep 7799
# 期望：LISTEN 0 128 *:7799

# 测试 API
curl http://127.0.0.1:7799/api/status
# 期望：{"ok":true,"data":{"workers":[...],...}}

# 浏览器访问
# http://sync.yourdomain.com → 仪表盘页面
```

---

## 10. 日志查看

```bash
# 实时日志
tail -f /www/wwwroot/lingxing-sync/code/logs/stdout.log

# 宝塔面板：软件商店 → Supervisor → 对应进程 → 日志
```

---

## 11. 升级流程

```bash
cd /www/wwwroot/lingxing-sync

git fetch origin
git checkout main
git pull --ff-only origin main

cd code
make build

# 重启服务（Supervisor 会自动重启）
supervisorctl restart lingxing-sync

# 程序启动时自动执行 migrations/*.sql（幂等），无需手动导入

# 验证
supervisorctl status lingxing-sync
curl http://127.0.0.1:7799/api/status
```

---

## 12. 防火墙

端口 7799 **不对外开放**，只通过 Nginx 反代访问。
宝塔 → 安全 → 防火墙：确保 7799 **不在开放列表**中。

---

## 13. 配置更改后操作

| 更改类型 | 操作 |
|---|---|
| 修改 enabled / cron | 点击 UI → 系统设置 → 重新加载配置（无需重启）|
| 修改 account / endpoint URL / QPS | `supervisorctl restart lingxing-sync` |
| 新增接口（加配置+建表） | 见 `07-add-endpoint.md` → 重启 |
| 修改端口 7799 | 同时改 `config.yaml` + Nginx 配置 + 重启两者 |
