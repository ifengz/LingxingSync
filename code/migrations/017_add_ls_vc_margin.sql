-- VC 毛利日报原始表。
--
-- 接口：POST /basicOpen/vc/report/nppm/list
-- 请求：sid + startDate/endDate + offset/length。
-- 真实探针字段：sid、asin、date、netPureProductMargin。
-- sid 必须使用 VARCHAR：领星会把 18 位店铺 ID 作为 JSON number 返回，不能依赖 Go
-- 浮点解码后的响应值；正式配置通过 force_inject_params 使用请求 sid 保真。
CREATE TABLE IF NOT EXISTS ls_vc_margin (
    account_id             VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    sid                    VARCHAR(32)  NOT NULL COMMENT 'VC 店铺 ID（请求值保真）',
    asin                   VARCHAR(32)  NOT NULL COMMENT 'Amazon ASIN',
    `date`                 DATE         NOT NULL COMMENT '毛利日期',
    netPureProductMargin   DECIMAL(18,8) NULL COMMENT '净产品毛利率/值（领星原字段）',
    synced_at              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, asin, `date`),
    INDEX idx_date (account_id, `date`),
    INDEX idx_asin (account_id, sid, asin)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='VC 毛利日报 /basicOpen/vc/report/nppm/list';
