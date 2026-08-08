# 领星同步机（LingxingSync）

> 把领星 OpenAPI 数据定时拉下来落库，给 polabel2 等项目当只读数据底座。

---

## 项目定位

| 是什么 | 不是什么 |
|---|---|
| 领星数据的唯一同步入口 | 不是 polabel2 的替代品 |
| 给其他项目提供只读 MySQL 表 | 不提供业务逻辑和聚合计算 |
| 轻量、单进程、宝塔部署 | 不用 Docker，不用 BullMQ，不用队列 |
| 每接口独立 goroutine，一个挂不影响其他 | 不是分布式系统 |

---

## 文档索引

### 宪法层（`CLAUDE.md` + 下面 10 份编号文档）

改这些要单独 commit，发现需要改先停下报备（`CLAUDE.md §1.14`）。

| 文档 | 内容 |
|---|---|
| [00-overview.md](00-overview.md) | 本文件：项目定位、文档索引、快速启动、polabel2 接入方式 |
| [01-architecture.md](01-architecture.md) | 架构设计、Go 选型理由、Worker 核心循环、**禁止引入清单（§8）** |
| [02-database.md](02-database.md) | 所有表 DDL（系统表 + 数据表）、polabel2 消费契约 |
| [03-config.md](03-config.md) | config.yaml 完整格式、所有字段说明、账号 ID 规范 |
| [04-api.md](04-api.md) | HTTP REST API 规范（前端调用） |
| [05-ui.md](05-ui.md) | UI 设计规范，极细到每个按钮（Tailwind + Alpine.js）|
| [06-deployment.md](06-deployment.md) | 宝塔部署全流程（Go 安装、Supervisor、Nginx）|
| [07-add-endpoint.md](07-add-endpoint.md) | 新增领星接口同步：建表 → 加配置 → 重启 |
| [08-api-reference.md](08-api-reference.md) | 领星 OpenAPI 接入参考：三套认证、签名算法、分页终止判定、踩坑汇总 |
| [09-endpoint-contract.md](09-endpoint-contract.md) | 接入合同五格模板、完整示例、限流键规则、加接口速查清单 |

### 过程与参考文件（**不属宪法层**，可随手改、可与代码同 commit）

| 文档 | 内容 |
|---|---|
| [progress.md](progress.md) | 过程记录（追加式，按日期） |
| [findings.md](findings.md) | 调查结论与实证证据（追加式）——**排障先读这份** |
| [lessons.md](lessons.md) | 教训沉淀 |
| [task_plan.md](task_plan.md) | 阶段性任务计划 |
| [10-frontend-rework-flow.md](10-frontend-rework-flow.md) | 前端动线重建执行规格（T1–T6，部分条目已废止，**非宪法**） |
| [otherlingxinggithub.md](otherlingxinggithub.md) | 外部领星开源仓参考：签名算法、限流实测值、680 路径清单 |
| [LINGXING_API_INTEGRATION.md](LINGXING_API_INTEGRATION.md) | 领星接入手册（来自 polabel2 生产验证，外部资料） |
| [sync-field-source-map.md](sync-field-source-map.md) | 字段来源对照。⚠️ 用户指定作字段依据，但**其表名/workflow 是 polabel2 概念**（`sales_trend_daily`/`operations_tracking`/Spotter 源），本项目架构里不存在，只取「哪个领星字段对应什么」这层信息 |

---

## 5 分钟快速启动

```bash
# 1. 安装 Go（服务器 SSH）
wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile && source /etc/profile

# 2. 克隆项目
cd /www/wwwroot
git clone https://github.com/your-org/lingxing-sync.git && cd lingxing-sync

# 3. 配置
cp config.example.yaml config.yaml
nano config.yaml          # 填入 DB 密码 + 领星 app_key/secret

# 4. 建表 + 编译 + 启动
make migrate && make build
./lingxing-sync            # 前台验证，Ctrl+C 后改用 Supervisor

# 5. 浏览器访问
# http://服务器IP:7799 → 仪表盘
```

详细部署步骤见 [06-deployment.md](06-deployment.md)。

---

## 关键约定

1. **port 7799**：HTTP 服务固定端口，不漂移。
2. **单写者**：只有 EndpointWorker 写自己的 `sync_tasks` 行；外部只能发信号或 INSERT。
3. **进程内限流**：`rate.Limiter` 键 = `(quota_group, path)`，不入 DB；`bucket=1` 时强制串行。
4. **fail-loud**：API 返回格式异常 → 抛错记日志，不静默兜底。
5. **表结构与领星一致**：字段名不翻译，polabel2 直连只读账号读。
6. **加接口 = 加配置 + 建表 + 重启**，零代码改动，见 [07-add-endpoint.md](07-add-endpoint.md)。

---

## polabel2 接入方式

polabel2 配置只读 MySQL 账号直连 `lingsync` 库，砍掉自身同步逻辑，从 `ls_*` 表取数：

```sql
-- polabel2 只读账号权限（在 lingsync 服务器执行一次）
CREATE USER 'lingsync_ro'@'%' IDENTIFIED BY 'readonly_password';
GRANT SELECT ON lingsync.* TO 'lingsync_ro'@'%';
FLUSH PRIVILEGES;
```

```yaml
# polabel2 .env 或 config
LINGSYNC_DB_HOST=sync-server-ip
LINGSYNC_DB_USER=lingsync_ro
LINGSYNC_DB_PASSWORD=readonly_password
LINGSYNC_DB_NAME=lingsync
```

字段契约见 [02-database.md §4](02-database.md)。
