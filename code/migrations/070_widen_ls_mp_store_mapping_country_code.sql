-- MultiPlat 店铺映射的 country_code 真实响应不保证为 ISO 两位码；
-- raw 层保留上游原值，规范化只属于下游 Reader/转换层。
ALTER TABLE ls_mp_store_mappings
    MODIFY COLUMN country_code VARCHAR(128) NULL;
