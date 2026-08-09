-- 将 008 旧版“账号级无 sid”表保留为只读备份，再创建按 VC 店铺隔离的正式表。
-- 旧数据无法从响应反推店铺，禁止猜 sid；RENAME 到备份表完整保留。

SET @vc_sales_table_exists := (
    SELECT COUNT(*)
    FROM INFORMATION_SCHEMA.TABLES
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_sales_report'
);
SET @vc_sales_sid_exists := (
    SELECT COUNT(*)
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_sales_report' AND COLUMN_NAME = 'sid'
);
SET @vc_sales_backup_sql := IF(
    @vc_sales_table_exists = 1 AND @vc_sales_sid_exists = 0,
    'RENAME TABLE ls_vc_sales_report TO ls_vc_sales_report_legacy_unscoped',
    'DO 0'
);
PREPARE vc_sales_backup_stmt FROM @vc_sales_backup_sql;
EXECUTE vc_sales_backup_stmt;
DEALLOCATE PREPARE vc_sales_backup_stmt;

CREATE TABLE IF NOT EXISTS ls_vc_sales_report (
    account_id                 VARCHAR(32)   NOT NULL COMMENT '本系统内部账号 ID',
    sid                        VARCHAR(32)   NOT NULL COMMENT 'VC 店铺 id（请求参数回填）',
    asin                       VARCHAR(16)   NOT NULL DEFAULT '' COMMENT 'ASIN',
    `date`                     DATE          NOT NULL COMMENT '报表日期',
    shippedUnits               INT           NULL,
    customerReturns            INT           NULL,
    orderedUnits               INT           NULL,
    shippedRevenueAmount       DECIMAL(18,4) NULL,
    shippedRevenueCurrencyCode VARCHAR(8)    NULL,
    orderedRevenueAmount       DECIMAL(18,4) NULL,
    orderedRevenueCurrencyCode VARCHAR(8)    NULL,
    shippedCogsAmount          DECIMAL(18,4) NULL,
    shippedCogsCurrencyCode    VARCHAR(8)    NULL,
    synced_at                  DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, asin, `date`),
    INDEX idx_date (account_id, sid, `date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='VC 销量报表（按店铺日粒度）';

SET @vc_realtime_table_exists := (
    SELECT COUNT(*)
    FROM INFORMATION_SCHEMA.TABLES
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_realtime_sales'
);
SET @vc_realtime_sid_exists := (
    SELECT COUNT(*)
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_realtime_sales' AND COLUMN_NAME = 'sid'
);
SET @vc_realtime_backup_sql := IF(
    @vc_realtime_table_exists = 1 AND @vc_realtime_sid_exists = 0,
    'RENAME TABLE ls_vc_realtime_sales TO ls_vc_realtime_sales_legacy_unscoped',
    'DO 0'
);
PREPARE vc_realtime_backup_stmt FROM @vc_realtime_backup_sql;
EXECUTE vc_realtime_backup_stmt;
DEALLOCATE PREPARE vc_realtime_backup_stmt;

CREATE TABLE IF NOT EXISTS ls_vc_realtime_sales (
    account_id     VARCHAR(32)   NOT NULL COMMENT '本系统内部账号 ID',
    sid            VARCHAR(32)   NOT NULL COMMENT 'VC 店铺 id（请求参数回填）',
    asin           VARCHAR(16)   NOT NULL DEFAULT '' COMMENT 'ASIN',
    startTime      VARCHAR(32)   NOT NULL DEFAULT '' COMMENT 'UTC 窗口开始',
    endTime        VARCHAR(32)   NULL COMMENT 'UTC 窗口结束',
    localStartTime DATETIME      NULL COMMENT '站点本地窗口开始',
    localEndTime   DATETIME      NULL COMMENT '站点本地窗口结束',
    currencyCode   VARCHAR(8)    NULL COMMENT '币种',
    orderedRevenue DECIMAL(18,4) NULL COMMENT '下单金额',
    orderedUnits   INT           NULL COMMENT '下单件数',
    synced_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, asin, startTime),
    INDEX idx_asin (account_id, sid, asin)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='VC 实时销量报表（按店铺小时粒度）';
