# polabel2 全项目 Lingxing 数据来源矩阵

## 先纠正口径

前一版把“页面需要的组合结果”直接计成“需要新增的数据集”，得出了 17 个数据集的夸大结论。这里撤回该结论。

当前静态证据是：

- `code/migrations` 约 59 张唯一物理表，包含系统表、原始同步表、报告表和派生支持表；
- `code/config.yaml` 已有 50 个 endpoint，目标表去重后约 24 张；
- 当前固定数据目录已按本轮四页补到 12 个版本，但这代表“对外发布合同”数量，不代表物理表数量；
- polabel2 的多数页面是多个事实的组合读取，不应为每个页面复制一张宽表。

因此本项目的最小路径是：先把已有 Lingxing 原始事实按固定领域合同发布，再只为经真实上游验证的缺失事实建表。不得把页面本地状态当作 Lingxing 原始缺口。

## 边界

LingxingSync 负责所有来自 Lingxing 的同步事实：店铺、产品、Listing、销售、广告、库存、表现、流量、退货和 VC 订单等。polabel2 负责本地业务状态：用户权限、备注、跟踪池、附件、打印/箱规计算、人工处理状态和页面交互。这些本地状态不需要在 LingxingSync 重建。

页面可以组合多个固定数据集，但每个数据集必须固定来源、粒度、唯一键、字段类型和 `snapshot/changes` Reader；禁止用一张跨页面万能宽表代替事实表。

## 页面需要什么，当前缺什么

| 页面/领域 | 可复用的 LingxingSync 事实 | 真正缺口 | 是否需要新物理表 |
|---|---|---|---|
| FBA/SC Links、Sales Trend、Products Overview、Profit Simulator | `ls_stores`、`ls_sc_products`、`ls_sc_listing`、`ls_sc_sales_report`、`ls_sc_sales_revenue`、`ls_fba_inventory`、`ls_sc_refunds`、`ls_sc_performance_daily`、`ls_ad_*` | `fba-links-v1` 已固定 SC Listing + 日维指标的页面行；其他页面继续复用原始事实/`listing-daily-v1` | 否，复用现有表 |
| VC Links、移动端、VC 销售趋势 | `ls_vc_listing`、`ls_vc_sales_report`、`ls_vc_realtime_sales`、`ls_vc_traffic`、`ls_vc_inventory`、`ls_vc_margin`、`ls_ad_*` | `vc-links-v1` 已固定 VC 店铺 + ASIN 汇总；VC 交通仅发布领星实际返回的 `glanceViews` | 否，复用现有表 |
| Products 管理与产品概览 | `ls_sc_products`、SC/VC Listing、店铺表 | 产品主档与本地配置/供应商/附件的责任边界；缺字段必须逐字段核验 | 否；本地配置仍由 polabel2 保存 |
| Operations Log | 上述销售、库存、表现、广告、VC 流量/实时事实 | `operations-log-v1` 已发布日维领星事实；跟踪目标、备注、Spotter/手工历史仍是 polabel2 本地业务，LingxingSync 不伪造 | 否 |
| Procurement Orders（VC PO） | `ls_vc_orders`、`ls_vc_po_details`、`vc-po-detail-v1` | 已有真实 PO 响应；固定 Reader 提供头字段并原样保留 `items` JSON，消费侧自行拆行 | 否，复用现有 raw 表 |
| Procurement DF、Invoice | 当前无 LingxingSync 原始表 | 必须取得真实列表/详情响应，确认字段、分页、唯一键和历史覆盖 | 未验证前不建表 |
| Sync Center、Integrations | 店铺表、endpoint 配置和同步运行记录 | 固定健康/coverage Reader；不是页面复制一张业务宽表 | 通常否 |
| System Log、Users、Labels、箱规、物流计算 | 主要是 polabel2 本地表和人工输入 | 不属于 Lingxing 同步事实 | 否，不迁移为 Lingxing 原始表 |

## 最小实施批次

### A. 先核实真实物理事实合同

按领域冻结少量固定合同，而不是按页面建表：

1. 店铺、产品与 SC/VC Listing；
2. 日销售与退货；
3. 广告与表现；
4. 库存与 VC 流量/实时/毛利；
5. VC PO（已补 `vc-po-detail-v1` 固定 Reader）；
6. 同步运行与覆盖诊断。

现有 raw 表优先复用，不按页面复制宽表。PO 已从真实响应确认，使用 `ls_vc_po_details.items` 原样 JSON 发布；DF/Invoice 在 probe 前不创建任何表。

VC 库存和 VC 流量暂不发布：原始日期字段仍需统一可排序语义，库存真实样本也未完成核验。广告、销售日维的组合字段继续优先复用既有 `listing-daily-v1` 和 raw 表，不另建页面宽表。

### B. 再接入同步写入

DF、Invoice 只能用 LingxingSync 自己的真实 probe 冻结字段、空值和唯一键。校验通过后，才补单接口原始表和 endpoint；未知字段不能靠 DTO/测试样本补齐。

### C. 统一完成门禁

每个页面字段只有在“真实上游 -> raw 落库 -> 固定 Reader -> snapshot/changes -> polabel2 页面回显”闭环完成后，才能标记为 100%。迁移文件存在、代码注册、任务成功或本地页面有旧数据，都不算生产覆盖证据。

## 当前结论

不是“缺几十张业务表”。本轮排除 Invoice；当前 PO、FBA Links、VC Links 和可由领星事实提供的运营日志日维已各有独立固定 Reader。DF、运营跟踪/备注等不是已验证的 LingxingSync 原始接口，仍由 polabel2 本地维护；未完成生产 token 授权和四页逐页落库回显前，不宣称 100%。
