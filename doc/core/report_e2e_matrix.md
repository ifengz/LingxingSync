# 正式报告生产 E2E 台账

最后更新：2026-08-22

这是正式报告 E2E 的**唯一查询页**。`progress.md` 记录执行过程，`findings.md` 记录诊断细节；本表只记录每个报告类型是否已经有一笔真实、非零的生产闭环证据，避免把“代码已接入”或“最近一次任务状态”误说成 E2E 已通过。

判定规则：

- `PASS`：至少一笔自营生产任务完成创建、DONE、下载、严格解析、原始落库、SHA 留痕，以及该类型适用的对账/日维修正。
- `PASS_HISTORY_LATEST_FAILED`：历史上已有 PASS，但后续一次尝试失败；不能把最新失败隐藏为 SUCCESS。
- `BLOCKED_*`：尚无完整非零生产 E2E，不能报告 PASS。
- `raw-only` 报告不写 `listing_daily_metrics`；其对账为 0 是合同预期，不是验收缺失。

| 报告类型 | 正式 report_type | E2E 状态 | 生产证据 | 验收说明 |
| --- | --- | --- | --- | --- |
| Customer Returns | `GET_FBA_FULFILLMENT_CUSTOMER_RETURNS_DATA` | PASS | audit 49，35 raw | 日维退货值已纠正并验证 |
| Customer Shipment Sales | `GET_FBA_FULFILLMENT_CUSTOMER_SHIPMENT_SALES_DATA` | PASS_HISTORY_LATEST_FAILED | audit 16，561 raw | 8 天对账、65 日维报表值已验证；后续 audit 17 为 FATAL |
| FBA Manage Inventory | `GET_FBA_MYI_UNSUPPRESSED_INVENTORY_DATA` | PASS | audit 54，41 raw | 6 个库存日维字段已纠正并验证 |
| FBA Manage Inventory (All) | `GET_FBA_MYI_ALL_INVENTORY_DATA` | PASS | audit 52，44 raw | 6 个库存日维字段已纠正并验证 |
| Reserved Inventory | `GET_RESERVED_INVENTORY_DATA` | PASS | audit 50，13 raw | 4 个 reserved 日维字段已纠正并验证 |
| AFN Inventory | `GET_AFN_INVENTORY_DATA` | PASS | audit 32，64 raw | inventory_sellable 已纠正并验证 |
| AFN Inventory by Country | `GET_AFN_INVENTORY_DATA_BY_COUNTRY` | BLOCKED_SCOPE | 无生产 audit | 当前自营范围没有可用 EU SC 店铺，不能用 NA 替代 |
| Storage Fee Charges | `GET_FBA_STORAGE_FEE_CHARGES_DATA` | PASS | audit 60，252 raw | raw-only |
| Overage Fee Charges | `GET_FBA_OVERAGE_FEE_CHARGES_DATA` | BLOCKED_UPSTREAM | audits 78、80、88 均 CANCELLED | 当前自营 scope 的最新 audit 88 仍无 document、下载、SHA 或 raw；停止盲目重建 |
| Long-term Storage Fee Charges | `GET_FBA_FULFILLMENT_LONGTERM_STORAGE_FEE_CHARGES_DATA` | BLOCKED_UPSTREAM | audit 75 现有 task | 创建成功，但同一 task 查询持续返回业务 429；无 document、下载、SHA 或 raw。2026-08-18 最新续查在 SQL 门禁结果不可读时停止，未重试 |
| Customer Shipment Replacements | `GET_FBA_FULFILLMENT_CUSTOMER_SHIPMENT_REPLACEMENT_DATA` | PASS | audit 66，1 raw | raw-only |
| Reimbursements | `GET_FBA_REIMBURSEMENTS_DATA` | PASS | audit 62，62 raw | raw-only |
| Stranded Inventory | `GET_STRANDED_INVENTORY_UI_DATA` | PASS | audit 76，1 raw | raw-only |
| Estimated Fees | `GET_FBA_ESTIMATED_FBA_FEES_TXT_DATA` | PASS | audit 74，17 raw | raw-only；同一 task 表头修复后闭环 |
| Inventory Planning | `GET_FBA_INVENTORY_PLANNING_DATA` | PASS | audit 87，42 raw；document/SHA、rows_imported、raw_rows、distinct `row_number`/`row_sha256` 均一致 | raw-only；旧 audit 82 不计入本次统计 |
| Inbound Noncompliance | `GET_FBA_FULFILLMENT_INBOUND_NONCOMPLIANCE_DATA` | PASS | audit 65，3 raw | raw-only |
| Recommended Removal | `GET_FBA_RECOMMENDED_REMOVAL_DATA` | PASS | audit 79，2 raw | raw-only；audit 77 的 CANCELLED 历史保留 |
| Removal Order Detail | `GET_FBA_FULFILLMENT_REMOVAL_ORDER_DETAIL_DATA` | PASS | audit 69，19 raw | raw-only |
| Removal Shipment Detail | `GET_FBA_FULFILLMENT_REMOVAL_SHIPMENT_DETAIL_DATA` | PASS | audit 70，10 raw | raw-only |
| All Orders | `GET_FLAT_FILE_ALL_ORDERS_DATA_BY_ORDER_DATE_GENERAL` | PASS | audit 37，17 raw | raw-only |
| Fulfilled Shipments | `GET_AMAZON_FULFILLED_SHIPMENTS_DATA_GENERAL` | PASS | audit 56，65 raw | raw-only |

## 当前计数

- 当前 Runner 已接入：21 类。
- 当前已闭合下载报告核验：18/21。Inventory Planning 使用当前 audit 87 严格 raw 核验闭合；旧 audit 82 从当前统计剔除；Customer Shipment Sales 的历史 PASS/最新 FATAL 双事实继续保留。
- 待复验或未闭环：3 类，分别是 AFN Inventory by Country、Overage Fee Charges、Long-term Storage Fee Charges。
- 若按“最新任务为 SUCCESS”计数，Customer Shipment Sales 的后续 FATAL 需要单独保留，因此是 17/21；这不抹掉 audit 16 的历史 PASS。

## 当前 Runner 之外的正式候选

下列类型不计入上述 21 类，不能因为名字在 Amazon 文档中存在就加入当前创建链：

- `GET_FLAT_FILE_SALES_TAX_DATA`：官方合同不允许 request/schedule，不能走当前 create/query Runner。
- `SC_VAT_TAX_REPORT`：需要 EU 自营范围和 Tax Invoicing Restricted/RDT。
- `GET_VAT_TRANSACTION_DATA`：需要 EU/RDT，且返回跨店数据，当前单店审计归属不能猜。

## 更新规则

每次生产报告终态后立即更新本表：写明 audit、raw 行数、适用的对账结果，以及是否存在后续失败任务。不得只依据代码 allowlist、页面状态或一次创建成功更新为 PASS。历史过程文件保留旧 audit，但当前统计只纳入本轮认可的闭合证据。
