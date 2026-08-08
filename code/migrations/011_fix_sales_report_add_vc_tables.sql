-- 011：三件事，都是「接口合同凭空臆造」留下的坑，全部按实证响应校正。
--   A. 修 ls_sc_sales_report 主键：去掉 seller_sku（ASIN 维度下领星根本不返回这个字段）
--   B. 新建 ls_vc_traffic（VC 流量报表，4 字段）
--   C. 新建 ls_vc_orders（VC 订单列表，30 字段）
--
-- 探测来源：本地 probe 模式抓取的真实响应，非文档推断、非 SDK 推断。
-- 原则（CLAUDE.md §1.6）：列名 = 领星返回字段名，一字不改、不翻译、不加工。
--
-- ⚠️ 日期/时间列一律 VARCHAR，不用 DATE/DATETIME。原因见 010 的同款说明：
--    领星对无值日期返回空字符串 ""，STRICT_TRANS_TABLES 下写 DATE/DATETIME 会报
--    1292 Incorrect datetime value 导致整批 fail；且同一响应里日期格式并不统一
--    （有的带 T/Z，有的标准格式）。存原样字符串，消费方自己解析。

-- ---------------------------------------------------------------------------
-- A. ls_sc_sales_report：主键去掉 seller_sku
-- ---------------------------------------------------------------------------
-- 病根：配置用 extra_params.asin_type=1（ASIN 维度）。实测该维度下
-- /erp/sc/data/sales_report/asinDailyLists 只返回 6 个字段：
--     asin, currency_code, map_value, product_name, r_date, sid
-- 没有 seller_sku（那是 asin_type=2 的 MSKU 维度才有的字段）。
-- 而旧表把 seller_sku 建成 NOT NULL 主键列 → 通用 Upsert 写 NULL →
--     Error 1048 Column 'seller_sku' cannot be null
-- → 每一行都失败，任务恒 error、恒 0 行。实测报错见 sync_task_logs task_id=406。
--
-- 正确唯一键 = (account_id, sid, r_date, asin)：ASIN 维度下每店铺每天每 ASIN 一行。
--
-- 【为什么是 ALTER 而不是 DROP TABLE】
-- 对照 polabel2：它 87 个迁移里 DROP TABLE 出现 0 次、TRUNCATE 0 次，改结构一律
-- ALTER TABLE（范本 0042_alter_aggregate_column_to_json.sql / 0040）。本仓 006 当初
-- 写了裸 DROP，后果是每次重启清空库存表（实测 5079 行 → 0）。故这里原地 ALTER，
-- 数据一行不动。
--
-- 【为什么还要那三行 PREPARE】
-- polabel2 有 schema 版本表，每个迁移只跑一次，所以它能写裸 ALTER。本仓
-- migrate.go 是「每次启动全量重跑 + 任一句失败即 fail-loud 阻断开机」，而 MySQL
-- 不支持 DROP COLUMN IF EXISTS —— 裸 ALTER 第二次开机就会报错、服务起不来。
-- 这三行是这个重跑模型下最短的可行写法，不是迁移框架，不引入版本表。
--
-- 【上生产前必做一次检查】
--     SELECT COUNT(*) FROM ls_sc_sales_report;
-- 旧结构下 seller_sku 是 NOT NULL 主键列而领星从不返回该字段，任何写入都必然
-- 失败，所以这张表预期为 0 行。若非 0（说明曾用 asin_type=2 的 MSKU 维度写过数），
-- 去掉 seller_sku 后可能出现主键重复，ADD PRIMARY KEY 会 fail-loud 阻断启动——
-- 那种情况先人工确认数据取舍，不要让迁移自己决定。
SET @sr_has_seller_sku := (
    SELECT COUNT(*)
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_sc_sales_report'
      AND COLUMN_NAME = 'seller_sku'
);
-- 除主键外，这里还把 r_date / product_name / map_value 收敛到下方 CREATE 的定义。
-- 否则「老库 ALTER 出来的结构」和「新装库 CREATE 出来的结构」会不一致：
--   r_date       DATE→VARCHAR(32)：领星无值时返回 ""，DATE 列在 STRICT 模式下报 1292
--   product_name 256→512：实测品名长度接近 256，会被截断
--   map_value    DECIMAL→VARCHAR(32)：领星实测返回字符串 "10"，原样存不强转
SET @sr_sql := IF(@sr_has_seller_sku,
    'ALTER TABLE ls_sc_sales_report
        DROP PRIMARY KEY,
        ADD PRIMARY KEY (account_id, sid, r_date, asin),
        DROP COLUMN seller_sku,
        MODIFY r_date       VARCHAR(32)  NOT NULL COMMENT ''报表日期【站点时间】，原样字符串'',
        MODIFY product_name VARCHAR(512) NULL     COMMENT ''品名'',
        MODIFY map_value    VARCHAR(32)  NULL     COMMENT ''type 对应统计量：本表为销量（件数）''',
    'DO 0');
PREPARE sr_stmt FROM @sr_sql;
EXECUTE sr_stmt;
DEALLOCATE PREPARE sr_stmt;

CREATE TABLE IF NOT EXISTS ls_sc_sales_report (
    account_id    VARCHAR(32)   NOT NULL COMMENT '本系统内部账号 ID',

    -- 领星返回字段（实测 asin_type=1 只有这 6 个）
    sid           VARCHAR(32)   NOT NULL COMMENT '领星店铺编号',
    r_date        VARCHAR(32)   NOT NULL COMMENT '报表日期【站点时间】，原样字符串',
    asin          VARCHAR(32)   NOT NULL COMMENT 'ASIN',
    product_name  VARCHAR(512)  NULL     COMMENT '品名',
    currency_code VARCHAR(8)    NULL     COMMENT '币种',
    -- map_value 是「请求参数 type 对应的统计量」，一次请求只返回一种指标。
    -- 本表启用时 extra_params.type=2 → 语义为「销量（件数）」。
    -- 领星把它作为字符串返回（实测 "10"），故存 VARCHAR 原样，不强转数值。
    map_value     VARCHAR(32)   NULL     COMMENT 'type 对应统计量：本表为销量（件数）',

    synced_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, r_date, asin),
    INDEX idx_asin_date (account_id, asin, r_date),
    INDEX idx_date      (account_id, r_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'SC 亚马逊销量统计 /erp/sc/data/sales_report/asinDailyLists（ASIN 维度）';

-- ---------------------------------------------------------------------------
-- B. ls_vc_traffic：VC 流量报表
-- ---------------------------------------------------------------------------
-- 接口：POST /basicOpen/vc/report/traffic/list
-- 实测字段仅 4 个（probe 样本 400 行，字段集稳定）：asin, date, glanceViews, sid
-- 注意 glanceViews 是驼峰——领星这个接口族返回驼峰字段名，列名照抄，不转蛇形。
-- `date` 是 MySQL 保留字，建表与查询都必须反引号。
CREATE TABLE IF NOT EXISTS ls_vc_traffic (
    account_id  VARCHAR(32) NOT NULL COMMENT '本系统内部账号 ID',

    sid         VARCHAR(32) NOT NULL COMMENT 'VC 店铺 id',
    asin        VARCHAR(32) NOT NULL COMMENT 'ASIN',
    `date`      VARCHAR(32) NOT NULL COMMENT '报表日期，原样字符串',
    glanceViews BIGINT      NULL     COMMENT '浏览量（领星驼峰字段名，照抄）',

    synced_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, asin, `date`),
    INDEX idx_date (account_id, `date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'VC 流量报表 /basicOpen/vc/report/traffic/list';

-- ---------------------------------------------------------------------------
-- C. ls_vc_orders：VC 订单列表
-- ---------------------------------------------------------------------------
-- 接口：POST /basicOpen/platformOrder/vcOrder/pageList
-- 实测 30 个字段。唯一键取 local_po_number（doc/core/08-api-reference.md §6.3 的
-- 既定合同，polabel2 生产验证过）；另建 id / 时间索引便于消费方查。
--
-- purchase_order_sku_list 是数组（订单行项目），存 JSON 原样——通用 Upsert 的
-- normalizeUpsertValue 会把 slice/map 序列化成 JSON 字符串再写入。
-- 不在同步机里拆行：宪法明确「同步机不承担聚合/加工，那些留在消费侧」。
CREATE TABLE IF NOT EXISTS ls_vc_orders (
    account_id                   VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',

    -- 领星返回字段（按字母序，列名严格对齐 API）
    ack_status                   INT          NULL COMMENT '平台确认状态',
    ack_status_desc              VARCHAR(64)  NULL COMMENT '平台确认状态描述',
    ack_update_time              VARCHAR(32)  NULL COMMENT '确认更新时间（常为 null）',
    currency_code                VARCHAR(8)   NULL COMMENT '币种',
    currency_icon                VARCHAR(8)   NULL COMMENT '币种符号',
    customer_order_number        VARCHAR(64)  NULL COMMENT '客户订单号【DF 类型订单】',
    erp_warehouse_id             VARCHAR(32)  NULL COMMENT 'ERP 仓库 id',
    erp_warehouse_name           VARCHAR(128) NULL COMMENT 'ERP 仓库名',
    focus_party_id               VARCHAR(32)  NULL COMMENT '主体 id',
    gmt_create                   VARCHAR(32)  NULL COMMENT '创建时间',
    gmt_modified                 VARCHAR(32)  NULL COMMENT '修改时间',
    id                           VARCHAR(32)  NULL COMMENT '领星订单行 id',
    item_amount                  DECIMAL(18,4) NULL COMMENT '商品数量/金额',
    local_po_number              VARCHAR(64)  NOT NULL COMMENT '本地 PO 号（唯一键）',
    print_num                    INT          NULL COMMENT '打印次数',
    purchase_order_date          VARCHAR(32)  NULL COMMENT '订购时间',
    purchase_order_number        VARCHAR(64)  NULL COMMENT '平台订单号',
    purchase_order_process_state INT          NULL COMMENT '订单处理状态',
    purchase_order_sku_list      JSON         NULL COMMENT '订单行项目数组（原样 JSON，消费侧自行拆行）',
    purchase_order_state         VARCHAR(32)  NULL COMMENT '订单状态',
    purchase_order_type          INT          NULL COMMENT '订单类型：0 DF / 1 PO / 2 DI',
    remark                       VARCHAR(512) NULL COMMENT '备注',
    seller_name                  VARCHAR(128) NULL COMMENT '店铺名称',
    ship_window_start            VARCHAR(32)  NULL COMMENT '发货窗口开始',
    ship_window_time             VARCHAR(64)  NULL COMMENT '发货窗口',
    ship_windows_end             VARCHAR(32)  NULL COMMENT '发货窗口结束（领星拼写含 s，照抄）',
    shipment_confirm_status      INT          NULL COMMENT '发货确认状态',
    shipment_label_status        INT          NULL COMMENT '标签状态',
    total_price                  VARCHAR(32)  NULL COMMENT '订单总额（领星返回字符串，原样存）',
    vc_store_id                  VARCHAR(32)  NULL COMMENT 'VC 店铺 id',

    synced_at                    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, local_po_number),
    INDEX idx_id            (account_id, id),
    INDEX idx_store_date    (account_id, vc_store_id, purchase_order_date),
    INDEX idx_gmt_modified  (account_id, gmt_modified)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'VC 订单列表 /basicOpen/platformOrder/vcOrder/pageList';

-- ---------------------------------------------------------------------------
-- D. ls_vc_inventory：VC 库存报表
-- ---------------------------------------------------------------------------
-- 接口：POST /basicOpen/vc/report/inventory/list（必填 sid + view + offset/length）
--
-- 【字段来源与 A/B/C 不同，务必知情】
-- 这张表不是 probe 实测建的——本地两个账号、sourcing 与 manufacturing 两个视图
-- 都返回 total:0（该 VC 店铺当前无库存报表数据），拿不到样本。
-- 字段取自两处互相印证的既有定稿：
--   1. doc/core/sync-field-source-map.md:332 的官方响应契约
--   2. polabel2 code/packages/connectors/lingxing-erp/src/endpoints/vc-inventory.ts
--      的字段读取清单 + 其测试样例（ADR-0021 确认已迁到本 OpenAPI 路径）
-- 全列 NULL 可空：多建的列若领星始终不返回，就一直是 NULL，不影响写入。
--
-- ⚠️ 首次真正拿到数据后必须回看一次：通用 Upsert 只写「表里有的列」，领星返回了
--    而表里没有的字段会被静默丢弃。届时对 sync_task_logs 的响应核一遍字段全集，
--    缺列就补。这是本迁移唯一的已知不确定点。
--
-- 唯一键 (account_id, sid, date, asin)：与 ls_vc_traffic 同形（同一报表族，
-- 官方契约都含 sid/date/asin）。msku 官方字段表未列出、仅 polabel2 fixture 有，
-- 故只作普通列不入主键——重演 seller_sku 那种「拿不到的字段当主键」是本次要修的坑。
-- `date` 是 MySQL 保留字，反引号。驼峰列名照抄，不转蛇形。
CREATE TABLE IF NOT EXISTS ls_vc_inventory (
    account_id VARCHAR(32) NOT NULL COMMENT '本系统内部账号 ID',

    sid        VARCHAR(32) NOT NULL COMMENT 'VC 店铺 id',
    asin       VARCHAR(32) NOT NULL COMMENT 'ASIN',
    `date`     VARCHAR(32) NOT NULL COMMENT '报表日期，原样字符串',
    msku       VARCHAR(128) NULL    COMMENT 'MSKU（官方字段表未列，polabel2 有，留列不入键）',

    -- 数量类
    sellableOnHandInventoryUnits           BIGINT NULL COMMENT '可售在库数量',
    unsellableOnHandInventoryUnits         BIGINT NULL COMMENT '不可售在库数量',
    netReceivedInventoryUnits              BIGINT NULL COMMENT '净收货数量',
    unhealthyInventoryUnits                BIGINT NULL COMMENT '不健康库存数量',
    aged90PlusDaysSellableInventoryUnits   BIGINT NULL COMMENT '90 天以上可售库存数量',

    -- 比率 / 时长类
    sellThroughRate           DECIMAL(18,6) NULL COMMENT '售出率',
    receiveFillRate           DECIMAL(18,6) NULL COMMENT '收货满足率',
    vendorConfirmationRate    DECIMAL(18,6) NULL COMMENT '供应商确认率',
    averageVendorLeadTimeDays DECIMAL(18,6) NULL COMMENT '供应商平均交付天数',

    -- 金额类
    sellableOnHandInventoryCostAmount         DECIMAL(18,4) NULL COMMENT '可售在库成本',
    unsellableOnHandInventoryCostAmount       DECIMAL(18,4) NULL COMMENT '不可售在库成本',
    aged90PlusDaysSellableInventoryCostAmount DECIMAL(18,4) NULL COMMENT '90 天以上可售库存成本',
    unhealthyInventoryCostAmount              DECIMAL(18,4) NULL COMMENT '不健康库存成本',
    netReceivedInventoryCostAmount            DECIMAL(18,4) NULL COMMENT '净收货成本',

    -- 币种类（官方契约写 "cost currency fields"，polabel2 逐个列出了这 5 个）
    sellableOnHandInventoryCostCurrencyCode         VARCHAR(8) NULL,
    unsellableOnHandInventoryCostCurrencyCode       VARCHAR(8) NULL,
    aged90PlusDaysSellableInventoryCostCurrencyCode VARCHAR(8) NULL,
    unhealthyInventoryCostCurrencyCode              VARCHAR(8) NULL,
    netReceivedInventoryCostCurrencyCode            VARCHAR(8) NULL,

    synced_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, `date`, asin),
    INDEX idx_date (account_id, `date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'VC 库存报表 /basicOpen/vc/report/inventory/list';
