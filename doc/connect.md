# LingxingSync 下游项目接入说明

本文说明下游项目如何读取 LingxingSync 已发布的数据表，并将数据落入自己的数据库。

## 1. 接入边界

LingxingSync 只提供固定版本的数据集 HTTPS API：

```text
POST /api/v1/datasets/{dataset_id}/snapshot
POST /api/v1/datasets/{dataset_id}/changes
```

当前生产 API 基址是：

```text
https://sync.usfan.net
```

完整请求地址由该基址加上数据集路径组成，例如：

```text
https://sync.usfan.net/api/v1/datasets/fba-inventory-snapshot-v1/snapshot
```

这是 LingxingSync 全部下游项目共用的服务地址，不是某个下游项目专属的秘密。下游项目专属信息是 Bearer Token、获准的数据表和店铺 SID。

下游项目需要自己完成以下工作：

- 在自己的数据库中建立数据表；
- 调用 `snapshot` 完成首次全量装载；
- 保存分页游标和 `changes_cursor`；
- 定期调用 `changes`，按主键幂等写入；
- 处理网络失败、接口错误和服务重启后的续读。

以下行为不属于本合同：

- 直连 LingxingSync 的 MySQL；
- 提交任意表名、SQL、排序表达式或动态字段表达式；
- 使用 Lingxing OpenAPI Token、ERP `auth-token` 或 `X-Sync-Secret` 访问数据集；
- 要求 LingxingSync 连接或修改下游数据库；
- 通过 Webhook、消息队列或 SDK 接收数据推送。

## 2. 下游项目必须取得的信息

项目管理员创建下游项目后，应把下面的信息交给下游开发者：

| 信息 | 用途 |
| --- | --- |
| LingxingSync HTTPS 地址 | 当前生产基址为 `https://sync.usfan.net`；不能使用公网明文 HTTP、IP:端口、`127.0.0.1:7799` 或数据库地址 |
| `project_id` | 管理识别，不作为 API 密钥 |
| `token_id` | 管理端定位凭证；请求数据时不放入 Bearer 头 |
| Bearer Token | 请求 `snapshot` / `changes` 的唯一凭证，放入 `Authorization` 请求头 |
| `dataset_id` | 允许读取的数据表版本，例如 `listing-daily-v1` |
| 店铺 SID | 允许读取的 `store` 值；每次请求只能使用授权店铺 |
| 已发布字段清单 | 请求中的 `fields`，必须使用接入说明中列出的字段 |
| 固定 `CREATE TABLE` SQL | 在下游建立与数据表版本一致的表结构、字段类型和主键 |
| 数据粒度和历史边界 | 正确理解一行数据代表什么，以及哪些日期尚无历史数据 |
| 日期和分页上限 | 单次 snapshot 最长日期范围，以及允许的最大 `page_size`；未拿到上限时不要传 `page_size`，使用服务端默认 100 |

这些信息在管理页 `/dataset-fields` 的项目列表中通过“查看接入说明”获取。下载的 Markdown 是当前项目权限的固定快照；如果管理员后来修改了数据表或店铺授权，需重新获取接入说明。

HTTPS 地址的取得和核对方式：

1. 当前固定生产地址以本项目 `doc/deploy.md` 中的宝塔站点域名为准，即 `sync.usfan.net`；
2. 管理员确认该域名在宝塔已配置有效 TLS 证书，并反向代理到本机 `127.0.0.1:7799`；
3. 下游只保存并使用 `https://sync.usfan.net`，不要自行使用服务器 IP、宝塔面板地址或本机端口；
4. 若未来更换域名，管理员先完成 HTTPS 证书和反向代理切换，再更新本文件及下游项目的 `api_base_url`；Bearer Token、数据表和店铺授权不因此改变。

创建、修改或删除项目后，管理接口会返回 `need_restart: true`。同步机重启前，新的读取权限尚未正式生效。

## 3. 下游项目需要修改的内容

### 3.1 增加数据表配置

在下游项目中为每个 `dataset_id` 建立独立配置，至少包含：

```text
dataset_id
api_base_url
bearer_token
store
fields
local_table_name
cursor_file_or_table
```

不要把多个数据集拼成一张“通用数据表”。每个数据集的字段、粒度、主键和游标合同独立维护。

### 3.2 执行固定建表 SQL

直接使用管理页接入说明中的 SQL，在下游数据库执行一次。不要根据字段名自行猜 SQL 类型，也不要删除主键列。

下游表名可以作为本地实现细节，但表结构必须覆盖接入说明中的字段和主键。建议同时建立：

- 数据表主键对应的唯一索引；
- `updated_at` 索引；
- 游标持久化表，或可靠的游标文件。

以 `listing-daily-v1` 为例，其业务主键是：

```text
store + channel + asin + sku + business_date
```

其他明细/快照数据集的主键以接入说明中的 `PRIMARY KEY` 为准。

### 3.3 增加同步任务

下游项目应新增一个可重复执行的同步任务，职责只有三段：

1. 首次或重建时执行 `snapshot`；
2. 后续执行 `changes`；
3. 将数据行和游标作为一个一致性单元提交。

不要把游标只保存在内存中。进程重启、部署或异常退出后，必须能够从上一次已提交的位置继续。

## 4. 首次全量同步：snapshot

### 4.1 请求

```bash
curl -X POST https://sync.usfan.net/api/v1/datasets/<DATASET_ID>/snapshot \
  -H 'Authorization: Bearer <PROJECT_TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{
    "store": "<STORE_SID>",
    "date_from": "2026-08-01",
    "date_to": "2026-08-07",
    "fields": ["<PUBLISHED_FIELD_1>", "<PUBLISHED_FIELD_2>"]
  }'
```

约束：

- `store`、`date_from`、`date_to` 必填；
- 日期格式为 `YYYY-MM-DD`；
- 日期范围不能超过服务端配置的最大天数，默认是 31 天；
- `fields` 只能使用接入说明中已发布的字段；
- 未传 `page_size` 时服务端默认返回 100 行；传入时不能超过管理员交付的服务端上限；
- 同一轮 snapshot 的后续请求必须原样携带返回的 `next_cursor`。

若需要初始化的历史范围超过单次允许天数，按不重叠日期窗口逐段执行 snapshot。每一段都完成分页；全部窗口完成后，使用**第一段**最后一页的 `changes_cursor` 开始 changes，不能改用后续窗口的游标。这样可以覆盖初始化期间已经发布的变化，重复行由幂等写入处理。

### 4.2 分页

当响应 `has_more=true` 时，使用 `data.next_cursor` 请求下一页：

```json
{
  "store": "<STORE_SID>",
  "date_from": "2026-08-01",
  "date_to": "2026-08-07",
  "fields": ["<PUBLISHED_FIELD_1>", "<PUBLISHED_FIELD_2>"],
  "cursor": "<NEXT_CURSOR>"
}
```

游标是不透明值。下游不得解析、截断、排序或自行拼接游标。

每个中间页也必须把本页数据与 `next_cursor` 一起持久化。下游进程中断后，使用该 snapshot 游标、相同的店铺、日期范围和字段继续；snapshot 游标不能传给 changes。

### 4.3 完成首次装载

最后一页会返回 `changes_cursor`：

```json
{
  "ok": true,
  "data": {
    "schema_version": "<DATASET_ID>",
    "rows": [],
    "has_more": false,
    "changes_cursor": "<INITIAL_CHANGES_CURSOR>"
  }
}
```

只有在该页数据成功写入下游数据库后，才提交并保存 `changes_cursor`。后续增量同步从这个游标开始。

## 5. 后续增量同步：changes

### 5.1 请求

```bash
curl -X POST https://sync.usfan.net/api/v1/datasets/<DATASET_ID>/changes \
  -H 'Authorization: Bearer <PROJECT_TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{
    "store": "<STORE_SID>",
    "cursor": "<CHANGES_CURSOR>",
    "fields": ["<PUBLISHED_FIELD_1>", "<PUBLISHED_FIELD_2>"]
  }'
```

`changes` 不接受 `date_from` 或 `date_to`。它只返回该游标之后已经写入 LingxingSync 发布数据集的变化。

### 5.2 写入和游标提交顺序

每一页按以下顺序处理：

1. 请求一页数据；
2. 在下游事务中按数据表主键执行 `INSERT ... ON DUPLICATE KEY UPDATE` 或等价幂等写入；
3. 将响应中实际出现的字段和 `next_cursor` 与本页数据放在同一个事务中提交；
4. 提交成功后，下一次请求才使用新的游标。

如果请求失败或下游事务回滚，保留旧游标并重试同一页。重复收到同一页不能造成重复业务行。

有数据时使用响应中的 `next_cursor`；空页也要保存响应游标。服务端会在没有变化时返回原游标，不能因为 `rows=[]` 就跳过游标处理。

显式传入 `fields` 时，响应只含固定字段和所请求的业务字段。下游 upsert 只能更新响应中实际出现的列，未请求字段必须保留本地已有值，不能写成 `NULL`。

### 5.3 调度建议

下游项目可以使用自己的定时任务、后台任务或任务调度器。建议：

- 为每个 `dataset_id + store` 独立保存游标；
- 单次任务只处理一个数据集和一个店铺，便于失败重试；
- 发生 `401` 时停止重试并联系管理员核对 Token；
- 发生 `403` 时核对数据表和店铺授权；
- 发生 `400` 时核对字段、日期范围、`page_size` 和游标是否被错误复用；
- 发生网络错误或 `5xx` 时保留旧游标，稍后重试。

## 6. 响应行和本地写入

响应格式如下：

```json
{
  "ok": true,
  "data": {
    "schema_version": "listing-daily-v1",
    "rows": [
      {
        "store": "12534",
        "channel": "SC",
        "asin": "B000000001",
        "sku": "SKU-1",
        "business_date": "2026-08-01",
        "updated_at": "2026-08-12T10:20:30.123456Z",
        "sales_units": 3
      }
    ],
    "next_cursor": "<NEXT_CURSOR>",
    "has_more": false
  }
}
```

下游必须按接入说明中的字段类型落库：

- `updated_at` 是 UTC 时间，响应以带 `Z` 的 RFC3339 时间返回；下游按 UTC 保留微秒精度；
- `NULL` 保持为 `NULL`，不能用 `0` 或空字符串猜补；
- `is_provisional`、`verification_status` 等状态字段按原值保存；
- `schema_version` 必须与当前本地表版本一致；不一致时立即停止、回滚本页，不得写入旧表；
- `deleted_at` 如果返回 `null`，不能自行推断为删除事件。

`changes` 返回变化后的完整数据行，不是字段差值。它表示 LingxingSync 已经发现并写入的数据变化，不代表领星侧所有历史修正都已被发现。上游历史修正仍依赖 LingxingSync 的重叠同步或正式报表对账。

## 7. 认证、权限和安全

请求必须使用：

```http
Authorization: Bearer <PROJECT_TOKEN>
Content-Type: application/json
```

Bearer Token 只能通过 HTTPS 传输。下游项目应：

- 将 Token 放在密钥配置或密钥管理系统中，不写入 Git；
- 不在普通业务日志中打印完整 Token；
- 不把 Token 放在 URL、查询参数或前端页面；
- 不把 `X-Sync-Secret` 当作数据集 Token；
- 只请求已授权的数据表和店铺。

Token 同时受 `dataset_scopes` 和 `store_scopes` 限制。越权请求不会被静默裁剪，而是返回明确错误。

## 8. 版本和变更

已发布的数据表版本是不可变合同：

- 已发布字段不能删除、改名、改类型或改变语义；
- 字段集合发生任何变化，都必须由管理员注册新的数据集版本并重新生成接入说明；
- 破坏性变化必须发布新的数据集版本，例如 `listing-daily-v2`；
- 下游为新版本创建新表，完成 snapshot 回填后再切换读取路径；
- 旧版本表、旧数据和旧同步任务继续保留，直到下游确认完成迁移。

不要让下游程序根据字段名自动执行 `ALTER TABLE`。结构变化必须通过明确的版本切换完成。

## 9. 当前数据集说明

当前系统注册的数据集以管理页和接入说明为准，不能自行拼接未发布的数据表。常见数据集包括：

| 数据集 | 粒度/说明 |
| --- | --- |
| `listing-daily-v1` | `store + channel + asin + sku + business_date` 的 Listing 日维数据 |
| `return-reason-detail-v1` | 退货原因明细，按 `store + stable_key` 读取 |
| `order-shipping-address-detail-v1` | 订单配送地址明细，按 `store + stable_key` 读取 |
| `fba-inventory-snapshot-v1` | FBA 每日库存快照，按 `store + stable_key` 读取；历史从该版本部署后的成功同步开始累计 |

具体字段、SQL 类型、主键和店铺范围以当前项目下载的接入说明为准。

## 10. 接入验收清单

- [ ] 已取得 HTTPS 地址、Bearer Token、数据表、字段和店铺范围。
- [ ] 已在下游数据库执行固定 `CREATE TABLE` SQL。
- [ ] 主键和 `updated_at` 索引已存在。
- [ ] 首次 `snapshot` 已按 `next_cursor` 完成分页。
- [ ] 已在数据成功落库后保存 `changes_cursor`。
- [ ] `changes` 已实现幂等写入和游标同事务提交。
- [ ] 重启下游同步任务后能从持久化游标继续。
- [ ] 已验证 `401`、`403`、`400`、网络失败和重复页不会破坏数据。
- [ ] 已确认没有直连 LingxingSync 数据库、没有使用任意 SQL、没有把 Token 写入日志或代码仓库。
- [ ] 数据表版本变化时使用新版本表，不自动修改旧版本表结构。
