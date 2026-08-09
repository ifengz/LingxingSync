-- VC 报表两张真表：实时销量（小时粒度）+ 销量报表（日粒度）
-- 接口：POST /basicOpen/vc/report/realtimeSales/list（令牌桶容量 1）
--       POST /basicOpen/vc/report/sales/list（令牌桶容量 1）
--
-- 宪法 §2：列名 = 领星「返回结果」字段名（VC 为 camelCase，原样不改名不猜）。
-- 两接口均按 VC 店铺迭代；响应行不含 sid，由请求参数回填：
--   实时销量：粒度 = (sid, asin, startTime 小时窗)；
--   销量报表：粒度 = (sid, asin, date 日)。
-- 列名大小写须与 JSON 字段逐字一致，否则 UpsertRows 按名匹配失败→该列写 NULL。

-- 1. VC 实时销量报表（小时粒度）
--    startTime/endTime 是 UTC ISO8601（含 T 与 Z，如 2025-12-01T04:00:00Z），
--    MySQL DATETIME 不认这种格式，故用 VARCHAR 存原始值（保真、不猜、不触发写入失败）。
--    localStartTime/localEndTime 是站点本地时间（YYYY-MM-DD HH:MM:SS），DATETIME 可直存。
CREATE TABLE IF NOT EXISTS ls_vc_realtime_sales (
    account_id     VARCHAR(32)   NOT NULL COMMENT '本系统内部账号 ID',
    sid            VARCHAR(32)   NOT NULL COMMENT 'VC 店铺 id（请求参数回填）',
    asin           VARCHAR(16)   NOT NULL DEFAULT '' COMMENT 'ASIN',
    startTime      VARCHAR(32)   NOT NULL DEFAULT '' COMMENT 'UTC 窗口开始（ISO8601，如 2025-12-01T04:00:00Z）',
    endTime        VARCHAR(32)   NULL     COMMENT 'UTC 窗口结束（ISO8601）',
    localStartTime DATETIME      NULL     COMMENT '站点本地窗口开始',
    localEndTime   DATETIME      NULL     COMMENT '站点本地窗口结束',
    currencyCode   VARCHAR(8)    NULL     COMMENT '币种',
    orderedRevenue DECIMAL(18,4) NULL     COMMENT '下单金额',
    orderedUnits   INT           NULL     COMMENT '下单件数',
    synced_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, asin, startTime),
    INDEX idx_asin (account_id, sid, asin)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='VC 实时销量报表（按店铺小时粒度）';

-- 2. VC 销量报表（日粒度）；date 是 MySQL 保留字，列名与主键全程反引号。
CREATE TABLE IF NOT EXISTS ls_vc_sales_report (
    account_id                 VARCHAR(32)   NOT NULL COMMENT '本系统内部账号 ID',
    sid                        VARCHAR(32)   NOT NULL COMMENT 'VC 店铺 id（请求参数回填）',
    asin                       VARCHAR(16)   NOT NULL DEFAULT '' COMMENT 'ASIN',
    `date`                     DATE          NOT NULL COMMENT '报表日期',
    shippedUnits               INT           NULL     COMMENT '发货件数',
    customerReturns            INT           NULL     COMMENT '退货件数',
    orderedUnits               INT           NULL     COMMENT '下单件数',
    shippedRevenueAmount       DECIMAL(18,4) NULL     COMMENT '发货销售额',
    shippedRevenueCurrencyCode VARCHAR(8)    NULL     COMMENT '发货销售额币种',
    orderedRevenueAmount       DECIMAL(18,4) NULL     COMMENT '下单销售额',
    orderedRevenueCurrencyCode VARCHAR(8)    NULL     COMMENT '下单销售额币种',
    shippedCogsAmount          DECIMAL(18,4) NULL     COMMENT '发货成本',
    shippedCogsCurrencyCode    VARCHAR(8)    NULL     COMMENT '发货成本币种',
    synced_at                  DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, asin, `date`),
    INDEX idx_date (account_id, sid, `date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='VC 销量报表（按店铺日粒度）';
