# 其他领星 OpenAPI 开源仓参考

> 本文档只收录「对本 Go 同步机有实际借鉴价值」的外部领星仓,以及**与语言无关、可直接落地的事实**(签名算法、限流实测参数、接口路径清单)。
> 借鉴原则:Python 仓的代码组织 / async 框架 / 类设计对 Go 项目**无价值**;只有算法步骤、实测数值、接口契约这类语言无关内容才值得抄。Go 能直接对照代码的是 `QQiot/lingxing`。
> 维护方式:外部仓随时可能改名/删库,抄进来的事实要做成本仓内部的定稿,不要长期外链依赖。

---

## 1. 仓库清单

| 仓库 | 地址 | 语言 | 对本项目的价值 |
|---|---|---|---|
| **QQiot/lingxing** | https://github.com/QQiot/lingxing | **Go** | ⭐ 最高:同语言,AES-ECB 签名、token 获取/刷新可直接对照 `internal/api/` |
| **SongKehao/lingxing-sdk** | https://github.com/SongKehao/lingxing-sdk | Python | ⭐ 高:680 接口路径清单、签名算法、**限流实测参数**(见 §3) |
| **zach22-1999/lingxing-mcp** | https://github.com/zach22-1999/lingxing-mcp | Python | 中:签名实现与 SongKehao 互相印证;只读 MCP,无落库 |
| AresJef/LingXingApi | https://github.com/AresJef/LingXingApi | Python | 低:async client |
| codeYi/lingxing | https://github.com/codeYi/lingxing | PHP | 低 |

**关键结论**:这些仓**全部停在 API client / SDK 层**(调领星接口、返回数据),没有一个做了"定时拉 → 限流 → 落库 → 给消费方读"的完整同步后端。本项目的同步链路架构在开源领星生态里是独一份,没有现成的同步后端可抄。

---

## 2. 签名算法(三方一致,Go 直接对照 QQiot)

领星 OpenAPI 业务请求签名(三个独立实现互相印证,结论一致):

```
1. 规范化参数:
   - 参数按 key 字典序排序
   - 跳过空值
   - bool 转小写(true/false)
   - dict/list 转 JSON(排序键)
   - 拼成  k=v&k=v
2. md5 = MD5(规范化串).hexdigest().upper()   ← 关键:MD5 后转大写
3. sign = AES-ECB-Encrypt(plaintext=md5, key=app_key)
   - 模式:ECB
   - padding:PKCS7
   - 密钥:app_key 本身(不是 app_secret),长度须为 16/24/32 字节
   - 输出:base64
```

- **Go 直接对照**:`QQiot/lingxing` 的 `aes.go`(标准 PKCS7 + ECB)、`authorization.go`(token 获取/刷新)。本项目 `internal/api/sign.go` 可逐行对照,确认 padding 与密钥用法无偏差。
- **Python 印证**:`SongKehao/lingxing-sdk` 的 `src/lingxing/core/sign.py` + `aes.py`;`zach22-1999/lingxing-mcp` 的 `lib/lingxing_openapi/client.py::generate_sign`。
- **注意**:token 获取接口(`/api/auth-server/oauth/access-token`)用 query 传 `appId` / `appSecret`,**不走上面的签名**;只有业务接口才需要 `app_key AES 签名`。

---

## 3. 限流实测参数(可直接进 `config.yaml` 的 `rate` 块)

> 来源:`SongKehao/lingxing-sdk` 的 `src/lingxing/core/rate_limiter.py`(实测注释)。这是本项目 `findings.md` 里一直没坐实的部分,现以下面数值为准。

| 场景 | 实测限流 | 建议间隔 | 领星错误码 |
|---|---|---|---|
| **Token 请求**(`/access-token`、`/refresh`) | 约每小时重置 | **≥ 60 秒** | `3001008` `"new requests too frequently. please request later."` |
| **业务 API** | 100 次/分钟 | **≥ 0.6 秒** | — |

落地建议:
- `rate.bucket=1` 的接口:`interval_ms` 取 `≥ 1000`(对应业务 API 留足余量,0.6s 是下限)。
- Token 由 TokenHolder 按 `app_key` 单飞刷新(本项目已实现),天然满足 60 秒间隔,不会多 Worker 并发踩 `3001008`。
- 撞 `3001008` 应**指数退避重试**,而不是直接把任务标 `error`——这是本项目 Worker 主循环可以补的一个改进点。
- 检测当前出口 IP 的官方接口:`https://toolbox.lingxing.com/api/getIp`(本项目部署到云服务器后出口固定,白名单一次性配置即可,不需要任何漂移处理)。

---

## 4. 接口路径清单(SongKehao 已覆盖 680 路径)

`SongKehao/lingxing-sdk` 的 `API-COVERAGE-REPORT.md` 标注了 **680 个 API path / 269 个 endpoint 方法**,且每个都带真实测试状态(OK / ERR:400 / ERR:500)。覆盖 VC 报表、海外仓、多平台广告、库存对账单等。

价值:本项目「加接口」时(`doc/core/07-add-endpoint.md` 五步),可用它快速查到目标接口的 `path` / `method` / 必填参数,避免从领星文档逐个翻。地址:
- 仓库:https://github.com/SongKehao/lingxing-sdk
- 覆盖报告:仓库根 `API-COVERAGE-REPORT.md`

> 注意:它的 OK/ERR 状态受其测试账号权限影响(如 VC 报表需 `vc_store_id`),ERR 不代表接口本身不可用,只代表该仓的测试条件没满足。以领星官方文档 `https://apidoc.lingxing.com/` 为最终准绳。

---

## 5. Go 项目能借鉴什么、不能借鉴什么

**能借(语言无关):**
- §2 签名算法 —— 已是定稿,对照 QQiot Go 版校验本项目实现即可。
- §3 限流实测数值 —— 直接进 `config.yaml` 的 `rate` 块。
- §4 接口路径清单 —— 加接口时查 path/参数。
- QQiot 的 Go 代码结构(token 刷新、http client 封装)—— 同语言,可对照写法。

**不能借(语言/范式不匹配,或违反本项目宪法):**
- Python async / 类继承体系 —— Go 是 goroutine + interface,套不上。
- MCP 项目的 token 缓存文件、多人令牌模式 —— 本项目用进程内 TokenHolder 单飞,不需要持久化。
- 任何仓的「完整同步链路」—— 它们没做;同步后端的参考在 Singer-SDK / Airbyte(通用 ETL 生态,见下),但那些是重量级框架,**引入即违反 CLAUDE.md「轻量、不锁」红线**,只学思想不引代码。

---

## 6. 同步后端(非领星)的设计参考,仅作思想借鉴

领星生态没有同步后端可抄。如果要看「增量游标 / 断点续传 / 父子流隔离」这些同步后端通式,**不要在领星仓里找**,看通用 ETL:

- **Airbyte 的 Amazon SP-API connector**:https://github.com/airbytehq/airbyte/tree/master/airbyte-integrations/connectors/source-amazon-seller-partner —— 声明式 YAML 把游标字段、回看窗、分页、429 退避、父子流全表达出来,是本项目「加接口零代码」理念的高级版。
- **meltano/singer-sdk**:https://github.com/meltano/sdk —— Stream 基类的 state/bookmark 断点续传、partition 级失败隔离。

⚠️ 这两个都是**重量级运行时框架**,只借鉴设计思想(`cursor_field` / `lookback_window` / 429 指数退避 / partition state),**不引入代码**——会违反本项目的轻量红线。

## 7. 2026-08 官方增量与报告导出交叉核对

Amazon 源报表还有生成频率边界：日报通常不超过每 4 小时生成一次，近实时报告不超过每 30 分钟生成一次。因此“正式报表自动下载”不能等同于每次调度都创建任务；同一店铺/报表/范围需要任务状态与最小间隔控制，失败时保留审计，不静默重复轰炸上游。

官方文档的结论必须按接口拆开，不能抽象成一个全局 `modified_since`：

| 数据 | 官方时间参数 | 同步策略 |
|---|---|---|
| Amazon 订单 | `date_type=2` 订单修改时间；`date_type=3` 平台更新时间 | 更新时间窗口增量 |
| Listing | `pair_update_*` 或 `listing_update_*` | 配对/Listing 更新时间增量 |
| FBA 退货 | `date_type=2` 更新时间 | 更新时间窗口增量 |
| VC PO | `search_field_time=3` 订单更新时间 | 更新时间窗口增量 |
| SC 日销量 | 单日 `event_date` | 重拉日期窗口 |
| 产品表现 | `start_date/end_date`，最多 92 天 | 按天请求形成日维 |
| SP/SD/HSA | 单日 `report_date`；活动级返回没有 ASIN/SKU | SP/SD 商品级按 ASIN 聚合；HSA/SB 保留店铺级，不能伪造 ASIN |
| Review/Feedback | `reviewDetail` / `feedbackDetail` 按日期查询 | 可形成新增量日维；不能把区间总量当每日新增 |
| VC 销量/流量/库存 | `startDate/endDate` 业务日期 | 重拉日期窗口 |

库存不能统一理解为一个数值。领星官方已注明旧 [`dailyInventory`](https://apidoc.lingxing.com/docs/SourceData/DailyInventory.md) 在 2023-12-01 后停更；SC 当前库存只能在 Sync 运行当天沉淀为快照，历史库存可另用官方 [`summaryQuery`](https://apidoc.lingxing.com/docs/Finance/summaryQuery.md) 或正式 Amazon Inventory Ledger 报表核对。VC [`inventory/list`](https://apidoc.lingxing.com/docs/Statistics/vcInventoryList.md) 本身返回 `date`、可售、不可售、90 天以上、unhealthy、收货满足率、售罄率、供应商确认率、平均交付周期及成本币种，可直接进入日维事实字段。

公开 SDK 仅作旁证，不能替代官方合同：

- [AresJef/LingXingApi 销售参数](https://github.com/AresJef/LingXingApi/blob/f9aa683f8cd7f63f2b448b32e7dac1a867fc7e82/src/lingxingapi/sales/param.py) 同时列出订单 `date_type=2/3` 与 Listing `listing_update_*`。
- [zach22-1999/lingxing-mcp 报告服务](https://github.com/zach22-1999/lingxing-mcp/blob/09372d54a71306124f20aae3f296db42e71c419b/lib/lingxing_openapi/services.py) 已实现报告创建、查询、续期和下载四个动作。
- [SongKehao/lingxing-sdk 报告接口](https://github.com/SongKehao/lingxing-sdk/blob/7db7d8472a6f303037bb13dbee103067c3516940/src/lingxing/endpoints/statistics.py) 交叉实现同一组三个 OpenAPI 路径。
- [Gemma-Analytics/ewah 的 FBA 退货配置](https://github.com/Gemma-Analytics/ewah/blob/0882d17d60a623ef526470efb207296ebb7822a7/ewah/hooks/amazon_seller_central.py) 明确该报告没有稳定业务主键，只能按报告任务与行号保留原始行，不能臆造 `return_date+order_id+sku` 唯一键。

最终准绳仍是领星官方文档：[订单](https://apidoc.lingxing.com/docs/Sale/Orderlists.md)、[Listing](https://apidoc.lingxing.com/docs/Sale/Listing.md)、[退货](https://apidoc.lingxing.com/docs/SourceData/RefundOrders.md)、[销量日报](https://apidoc.lingxing.com/docs/Statistics/AsinDailyLists.md)、[产品表现](https://apidoc.lingxing.com/docs/Statistics/AsinListNew.md)、[SP 活动报表](https://apidoc.lingxing.com/docs/newAd/report/spCampaignReports)、[SP 商品报表](https://apidoc.lingxing.com/docs/newAd/report/spProductAdReports)、[SB/HSA 活动报表](https://apidoc.lingxing.com/docs/newAd/report/hsaCampaignReports)、[SD 活动报表](https://apidoc.lingxing.com/docs/newAd/report/sdCampaignReports)、[VC 销量](https://apidoc.lingxing.com/docs/Statistics/vcSalesList)、[VC 库存](https://apidoc.lingxing.com/docs/Statistics/vcInventoryList)、[报告创建](https://apidoc.lingxing.com/docs/Statistics/reportCreateReportExportTask.md)、[报告查询](https://apidoc.lingxing.com/docs/Statistics/reportQueryReportExportTask.md)、[链接续期](https://apidoc.lingxing.com/docs/Statistics/AmazonReportExportTask.md)。

领星帮助中心还区分了网页下载能力：[跨站点广告下载](https://www.lingxing.com/help/article/downloadCenter) 是创建任务后由网页生成 Excel；[下载中心自动生成报告](https://www.lingxing.com/help/article/DownloadCenter1) 当前明确写的是利润报表。两者都不能当作通用第三方 API 合同，Sync 只接入上面的正式 OpenAPI 报告导出链路。
