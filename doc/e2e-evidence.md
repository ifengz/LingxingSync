# LingxingSync 生产验证台账

最后更新：2026-08-22

这份台账只记录已经执行并有运行态证据的结果。`未覆盖` 表示该范围尚未单独验收，**不表示系统故障或功能未实现**。

## 当前结论

LingxingSync 已部署并可向下游发布数据；生产已证明四张固定数据集对店铺 `12534` 存在真实数据。`stock` 已完成一条真实库存快照闭环：LingxingSync 返回数据，`stock` 写入自己的 MySQL。

## 上游同步接口闭合台账（生产）

证据全部来自生产 `https://sync.usfan.net` 的 `/api/config`、`/api/tasks` 和任务日志。

- 生产当前配置 54 个接口；54/54 个接口都有至少一笔历史 `success` 任务，证明请求、解析、落库和任务收尾曾经完整闭合。
- `sc_products_sc_us_1`、`vc_po_details_sc_us_2`、`sc_sales_orders_sc_us_2` 已在 `d26c072` 部署后单独生产 E2E 成功，任务分别为 15293、15294、15295。
- 三个任务分别完成 43/46/339 页、8477/46/64962 条；任务日志均为 HTTP 200，无 API 错误、无 `3001008`。

### 生产异常分类

判定标准：同一接口在至少 3 个独立调度批次出现同类错误，才记为重复故障证据；少于此门槛先记为波动观察。

| 接口 | 生产证据 | 当前判定 |
| --- | --- | --- |
| `sc_refunds_sc_us_1` | 2026-08-18、19、20、21 连续 4 次 `1205 Lock wait timeout` | 重复故障证据，纳入锁重试修复验证 |
| `ad_sp_product_sc_us_1` | 2026-08-13、16、18、19、20、21 多次 `1205/1213` | 重复故障证据，纳入锁重试修复验证 |
| `ad_sd_product_sc_us_1` | 2026-08-13、16、18、19、21 多次 `1205/1213` | 重复故障证据，纳入锁重试修复验证 |
| `ad_hsa_campaign_sc_us_1` | 2026-08-13、16、18、19、20、21 多次 `1205` | 重复故障证据，纳入锁重试修复验证 |
| `ad_sp_product_sc_us_2` | 2026-08-19、21 两次 `1205`，其间有成功任务 | 波动，暂不处理 |

上述历史异常最后一次发生在 `d26c072` 生产部署前。修复后生产复验已成功：`sc_refunds_sc_us_1` 任务 15348、`ad_sp_product_sc_us_1` 任务 15349、`ad_sd_product_sc_us_1` 任务 15350、`ad_hsa_campaign_sc_us_1` 任务 15351，均为 `success`，任务日志无 HTTP/API 错误。当前未发现这些异常是领星限流或 `3001008`。

### 2026-08-22 生产最终快照

- 生产 54 个接口：54/54 有历史闭合记录。
- 修复复验后：53 个接口最新任务为 `success`。
- `ad_sp_product_sc_us_2` 最新错误仍为历史任务 14478（2026-08-21），此前仅两次 `1205` 且有 14 次成功，按规则记为波动，不处理。
- 当前没有达到“多批次重复失败”门槛的未修接口。

### 最小修法依据

- 本项目采用的本地数据库锁处理与 polabel2 的 `fba-query-index` 做法一致：完整事务 rollback 后仅对 1205/1213 做 3 次 50/100/150ms 重试，不重新请求领星。
- polabel2 的上游限流方案是单 lane、capacity=1、最小间隔和远端冷却；本项目保留现有单 worker/内存 limiter，不引入 lane 表、队列、租约或分布式锁。
- GitHub SDK 的可借鉴部分仅是业务请求节奏（约 0.6 秒）和 token 请求单独节奏；不能把其 429/3001008 重试策略套到本次 1205/1213 数据库锁故障上。

## 已验证

| 链路 | 结果 | 运行态证据 |
| --- | --- | --- |
| LingxingSync 生产部署 | PASS | 生产 `deploy_commit` 为 `d26c072bcda916105175bcff33517781f7b1eb60`，`db_connected=true`。 |
| 管理页面与管理 API | PASS | 生产 `/api/config` 返回 2 个账号、54 个接口；`/api/tasks` 有真实任务数据；`/api/datasources/datasets/catalog` 有 4 个数据集和 1 个下游项目。 |
| 下游 guide 保护 | PASS | 匿名及错误管理密钥请求 `GET /api/datasources/datasets/projects/{token_id}/guide` 均返回 `401`，拒绝体只含 `ok/error`；正确管理密钥返回 `200`。 |
| 下游数据集匿名保护 | PASS | 匿名 `POST snapshot`、`POST changes` 均返回 `401`；这两个接口仍要求项目 Bearer Token。 |
| 固定数据集源数据 | PASS | 生产管理导出对店铺 `12534`、`2026-08-01..2026-08-17` 返回：Listing 1066 行、退货原因 308 行、订单地址 883 行、FBA 库存 44 行，均为 HTTP 200。 |
| `stock` 首次库存快照 | PASS | `stock` 生产接口调用 LingxingSync 后，店铺 `12534` 成功写入 132 行，返回 `hasMore=false` 与 `changesCursor=true`。 |
| 下游协议自动化 | PASS（本地） | `snapshot` 分页、最终页 `changes_cursor`、`changes` 续读、字段 allowlist 和 FBA 固定 SQL Reader 均有 Go 回归测试。它不是生产消费者 E2E。 |
| LingxingSync 本地代码门禁 | PASS（本地） | `go test ./... -count=1` 已通过；本次生产提交为 `d26c072`。它不替代生产同步验收。 |

## 已验证的真实闭环

```text
领星数据已落入 LingxingSync 发布数据集
  -> stock 使用项目 Bearer 拉取店铺 12534 的 snapshot
  -> stock 本地 MySQL 写入 132 行
```

`stock` 首次同步已经保存 `changes_cursor`，因此后续调用会进入增量读取路径，而不是重新执行全量快照。

## 未覆盖范围

| 范围 | 状态 | 说明 |
| --- | --- | --- |
| 其他店铺 | 未覆盖 | 本次 `stock` 真实请求显式指定店铺 `12534`；其他已授权店铺尚未逐店执行同一验证。 |
| `stock` 的真实 changes 落库 | 未覆盖 | 已保存初始游标，但尚未记录一次“上游产生变化 -> LingxingSync 落库 -> stock changes 幂等写入”的独立运行证据。 |
| 其他下游项目 | 未覆盖 | 当前有 `stock` 这一真实消费者闭环；其他项目需各自完成一次 snapshot 后才可记为 PASS。 |
| 全部启用上游接口 | PASS（历史闭合） | 生产 54/54 个接口都有历史 `success` 任务；2026-08-22 修复复验后 53 个最新成功，`ad_sp_product_sc_us_2` 的两次 1205 按波动观察。 |
| 下一次自动部署健康检查 | 待配置 | 当前工作区已有健康检查认证调整，但尚未作为新提交部署验证；本项不影响已记录的生产同步结果。 |

这些是验收范围，不是当前待修 bug。只有出现请求报错、数据未落库、游标不推进或页面读不到本地数据时，才按故障处理。

## 证据来源

- LingxingSync 生产数据集导出记录：[progress.md](core/progress.md) 的 2026-08-17 下游项目接入说明与固定建表合同条目。
- `stock` 下游闭环记录：Codex 任务“检查远端同步状态”，生产接口返回 `success=true`、132 行、无下一页并保存增量游标；该证据属于 `stock` 项目，不伪装成本仓测试结果。
- 协议与鉴权回归：[`handler_test.go`](../code/internal/datasetapi/handler_test.go)、[`detail_readers_test.go`](../code/internal/datasetapi/detail_readers_test.go)、[`handlers_dataset_tokens_test.go`](../code/internal/server/handlers_dataset_tokens_test.go)。

## 相关台账

- 各类领星正式报告的独立生产 E2E 状态见 [report_e2e_matrix.md](core/report_e2e_matrix.md)。该表按报告类型记录 PASS、上游限流/取消和未闭环项，不能由本页 `stock` 库存快照结果替代。
- 下游项目接入方式见 [connect.md](connect.md)。
