# 日维预览与正式报表 UI 设计

## 已确认范围

- `/sync` 页标题旁增加一个可键盘聚焦的问号提示。
- `/sync` 现有页面内增加“正式报表校验”卡片，不增加导航或新页面。
- `/datasources` 现有页面内增加“日维数据预览”卡片，不改变下游字段配置合同。

## 交互设计

### 修改时间提示

问号悬停、聚焦或点击显示两组说明：

- 支持按修改时间发现历史修正：亚马逊订单、Listing、FBA 退货、VC PO。
- 不支持统一修改时间：销量、Performance、SP、SD、HSA、VC 销量/库存；这些按最近业务日期重拉。

### 日维数据预览

筛选项固定为开始日期、结束日期、店铺、ASIN、SKU；日期必填，其他可空。结果直接读取固定 `listing_daily_metrics` 与 `listing_dimensions`，按业务日期倒序分页。页面展示身份键、销量/销售额/退货、库存、Performance、广告和验证状态；数据库 NULL 显示短横线，不显示 0。

### 正式报表校验

首期只展示已实现的 FBA Customer Returns。配置项固定为启用、账号、seller、store、region、marketplace、cron、最近完整日窗口。保存复用现有 `ConfigStore.Save` 与 `Scheduler.Rebuild` 热加载。

状态区域读取 `ls_report_export_tasks` 与 `listing_daily_reconciliations`，展示最新任务、下载行数、错误和三类差异数量。未配置显示“未启用”；缺表或查询失败显示明确错误，不伪造成功。

## 数据边界

- 不接受表名、SQL、排序表达式或动态字段表达式。
- 不增加新表、迁移、锁、队列、服务或依赖。
- 正式报表凭证继续使用 OpenAPI `app_key/app_secret/access_token/sign`，不混用 ERP auth-token。
- 当前真实领星报表下载 E2E 仍为 UNKNOWN，UI 必须如实展示最近数据库任务，不宣称链路已通。

## 验证

- 新 API 和 Alpine 行为先写失败测试。
- 全量 Go、race、vet、build、Node 检查通过。
- 真实本地 MySQL 固定查询验证。
- 7799 桌面与移动截图检查 tooltip、两张卡片、空态和无重叠。

## 实施任务

### Task 1：后端固定管理接口

- 新增固定日维预览 GET API：日期必填，店铺/ASIN/SKU 可选，`page/page_size` 有上限；只查询 `listing_daily_metrics` 和 `listing_dimensions`。
- 新增正式报表配置 GET/PUT API，只接受当前 `fba_customer_returns` 合同，复用配置校验与热加载。
- 新增正式报表最近任务/对账状态 GET API，只读既有两张审计表。
- 先写会因路由/行为缺失而失败的 handler 与 SQL 测试，再做最小实现。

### Task 2：现有页面内 UI

- `/sync` 标题旁增加修改时间 tooltip。
- `/sync` 增加正式报表配置和最近状态卡片。
- `/datasources` 增加日维数据预览筛选、表格、分页与空/错状态。
- 先写会因组件行为缺失而失败的 Node/模板测试，再做最小 Alpine/HTML 实现。

### Task 3：宪法文档与集成收口

- 单独更新 `doc/core/04-api.md` 与 `doc/core/05-ui.md`，准确记录新增固定管理入口和页面卡片。
- 执行全量门禁、真实本地 MySQL、浏览器桌面/移动检查和独立代码审查。
- 文档和业务代码按用户要求分组提交，不 push、不部署。
