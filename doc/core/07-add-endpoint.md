# 领星同步机 — 新增接口指南（宪法层）

> 新增一个领星接口同步：填接入合同 → 建表 → 加配置 → 重编译 → 重启。零代码改动。
>
> 每格信息来源：**领星官方接口文档页**，不是 SDK、不是猜测。详见 [09-endpoint-contract.md](09-endpoint-contract.md)。

---

## 两条路：先看你是不是要接「清单里已有的接口」

**路 A —— 从清单启用（不懂技术者走这条，零风险）：**
`/sync` →「定时调度」Tab →「从清单添加接口」→ 挑一个 → 选账号 → 点「启用」→ 重启。
表已由 migrations 预建，path/唯一键/参数/限流都已配死并测过，**不碰 SQL/JSON/表名**。
清单来源：`internal/config/catalog.go` 的 `catalogEntries`。

**路 B —— 手动填合同（清单里没有的新接口，开发者走这条）：**
就是下面的完整流程。做完后建议把它「加进清单」（catalog.go + 建表迁移），让下次变成路 A。
UI 上路 B 收在「高级 / 开发者」折叠区，默认隐藏，避免非技术用户误用。

> 参数形态三选一（config 字段，路 A 的模板也用同一套）：
> `window_days>0` 注入 `start_date/end_date` 范围；`date_field` 注入单日期（如 `event_date`，配 `date_offset_days`：1=昨天）；都不填=全量。

---

## 步骤总览

```
0. 打开领星官方文档，找到目标接口页，填五格接入合同
1. 写迁移 SQL（新表）
2. 执行迁移
3. 在 config.yaml 添加一段 endpoint 配置（按合同五格）
4. 重编译并重启
5. 手动触发验证
```

---

## Step 0：填接入合同（领星官方文档 → 五格）

打开该接口的官方文档页，抄写：

| 格 | 来源 | 填入 |
|---|---|---|
| path + method | 文档标题区 | config `url` |
| 请求参数 | Request Parameters 表 | `extra_params` + 内置分页参数 |
| 幂等键 | Response 唯一标识字段 | `record_id_fields` |
| 限流档案 | Rate Limit 区块 | config `rate:` 块（原样抄，不猜） |
| 目标表结构 | Response Data 表 | 建表 DDL 列名 |

⚠️ `rate.bucket = 1` 的接口只能**串行翻页**，不能并发。

完整示例见 [09-endpoint-contract.md §产品表现示例](09-endpoint-contract.md)。

## Step 1：写迁移 SQL

在 `migrations/` 新建文件，命名规则 `00N_add_{table}.sql`（N 递增，唯一，不重复）：

```sql
-- migrations/003_add_ls_settlements.sql
CREATE TABLE ls_settlements (
    settlement_id     VARCHAR(64)    NOT NULL,
    account_id        VARCHAR(32)    NOT NULL,
    store_id          VARCHAR(32)    NULL,
    store_name        VARCHAR(128)   NULL,
    transaction_type  VARCHAR(64)    NULL,
    order_id          VARCHAR(64)    NULL,
    sku               VARCHAR(128)   NULL,
    amount            DECIMAL(14,4)  NOT NULL DEFAULT 0,
    currency          VARCHAR(8)     NULL,
    settlement_date   DATE           NULL,
    gmt_modified      DATETIME       NULL,
    synced_at         DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP
                                     ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, settlement_id),
    INDEX idx_store_date (account_id, store_id, settlement_date),
    INDEX idx_order      (account_id, order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**规范**：
- 表名 `ls_{endpoint_type}`（全小写下划线）
- 必须有 `account_id` + 业务主键，构成联合 PRIMARY KEY
- 必须有 `gmt_modified` + `synced_at`
- 字段名与领星 API 返回字段名一致（不翻译，不重命名）

---

## Step 2：执行迁移

```bash
# 生产环境（宝塔服务器 SSH）
mysql -u lingsync_rw -p lingsync < migrations/003_add_ls_settlements.sql

# 验证
mysql -u lingsync_rw -p lingsync -e "DESCRIBE ls_settlements;"
```

---

## Step 3：在 config.yaml 添加 endpoint 段

```yaml
endpoints:
  # ... 已有接口保持不变 ...

  - name: sc_settlements         # 唯一标识，字母+下划线
    display: "SC 结算报告"
    account: "sc_us"             # 必须是 accounts[].id 中存在的值
    # ⚠️ path 不要带 /openapi 前缀：baseURL 本身已是 https://openapi.lingxing.com，
    # 写成 "/openapi/erp/..." 会拼成 /openapi/openapi/... → 领星回
    # HTTP 200 + code=500 + msg="404 NOT_FOUND"（历史上 sc_sales_orders /
    # sc_inventory 都栽在这里）。领星文档的 "API Path" 一般形如 /erp/sc/...，原样抄即可。
    # 另：本段 sc_settlements 只是格式示例，path/唯一键均未经实证，勿直接照抄使用。
    path: "/erp/sc/data/settlements/list"       # 从领星文档"API Path"原样抄（不加 /openapi）
    method: "POST"                              # 从领星文档"请求方式"原样抄
    table: "ls_settlements"      # 上一步建的表名
    record_id_fields: ["settlement_id"]  # 唯一键字段数组（来自官方文档"唯一键说明"）
    rate:                        # 从领星官方文档 Rate Limit 区块原样抄
      bucket: 3
      interval_ms: 333
      multi_interval_ms: 2000
      dimension: "account+path"
    cron: "0 2 * * *"            # 每天凌晨 2:00
    enabled: true
    window_days: 30              # 拉近 30 天数据
    extra_params:                # 领星接口特有参数（可选）
      report_type: "standard"
```

---

## Step 4：重编译并重启

```bash
cd /www/wwwroot/lingxing-sync

# 重编译
make build

# 重启 Supervisor 守护进程
supervisorctl restart lingxing-sync

# 确认启动正常
supervisorctl status lingxing-sync
```

---

## Step 5：手动触发验证

```bash
# 触发新接口同步一次（不等 cron）
curl -X POST http://127.0.0.1:7799/api/sync/sc_settlements

# 查看任务状态
curl "http://127.0.0.1:7799/api/tasks?endpoint=sc_settlements&page_size=1"
```

或在 UI → 仪表盘 / 接口列表 找到新接口卡片 → 点击「立即同步」→ 查看历史确认成功。

---

## 常见问题

**Q：领星 API 有分页参数怎么配？**
A：Go Worker 内置通用分页逻辑（`page`, `page_size`），按领星 OpenAPI 标准自动处理。接口特有参数用 `extra_params` 传入。

**Q：领星返回嵌套 JSON 怎么办？**
A：先在测试环境调用一次，确认返回字段结构，展开一层存进表里。嵌套对象如有需要，用 JSON 列暂存，后续按需展开。

**Q：某个字段返回值可能为 null 怎么办？**
A：表中该列 NOT NULL 改为允许 NULL。fail-loud 只针对「必须存在的唯一键字段」；其他字段 null 正常入库。

**Q：record_id_field 是数组字段（一个响应多条记录）怎么配？**
A：领星接口通常是 `data.list[]`，每个 item 里有唯一键。`record_id_field` 填这个 item 内的字段名，如 `"order_id"`。

**Q：两个账号要同步同一接口怎么配？**
A：配两个 endpoint 段，name 不同（`sc_sales_orders_us` 和 `vc_sales_orders_de`），table 可以相同（靠 `account_id` 列区分），也可以不同。
