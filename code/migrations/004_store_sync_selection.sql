-- 004_store_sync_selection.sql
-- store_sync_selection：店铺「是否参与后续同步」的账号级选择表（系统表，非 ls_* 数据表）。
--
-- 为什么单独建表而不在 ls_stores 加列（宪法 §9 / CLAUDE.md §3）：
--   ls_stores 是 ls_* 数据表，由通用 Upsert 落库，ON DUPLICATE KEY UPDATE 会覆盖
--   除主键外「表的全部列」。领星店铺列表响应里没有「是否参与同步」这个字段，
--   若把该开关塞进 ls_stores，每次同步店铺列表都会把用户勾选覆盖成 NULL/默认值。
--   故按 sync_tasks 的模式另立系统表：HTTP handler 单独写，worker 只读，
--   ls_* 保持「列名=领星字段名」纯净，通用 Upsert 一个字都不用改。
--
-- 语义（与 endpoint.store_sids 白名单一致的「空=全放行」约定）：
--   某 account 在本表「一行都没有」   → 视作从未配置 → 该账号全部店铺照常同步（向后兼容，部署不静默停同步）。
--   某 account 「至少有一行」         → 已配置 → 只同步 enabled=1 的 sid；enabled=0 或本表缺行的新店铺都不同步。
-- 因此保存时对该账号「每个已知店铺」都写一行（勾选 enabled=1 / 未勾选 enabled=0），
-- 用「有没有行」区分「未配置」与「配置了但全不选」。
CREATE TABLE IF NOT EXISTS store_sync_selection (
    account_id VARCHAR(32) NOT NULL COMMENT '本系统内部账号 ID',
    sid        VARCHAR(32) NOT NULL COMMENT '领星店铺编号',
    enabled    TINYINT     NOT NULL DEFAULT 1 COMMENT '1=参与后续同步，0=不参与',
    updated_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
