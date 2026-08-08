package api

import "fmt"

type storeField struct {
	source string
	target string
}

type storeMapping struct {
	storeType string
	fields    []storeField
}

var storeMappings = map[string]storeMapping{
	"/erp/sc/data/seller/lists": {
		storeType: "SC",
		fields: []storeField{
			{source: "sid", target: "sid"},
			{source: "name", target: "store_name"},
			// 领星 SC 接口同时返回 region(大区，如 NA)和 country(商城所在国家名称，如"西班牙")。
			// 国家列取 country，不能取 region，否则北美区所有店铺都显示 NA。
			{source: "country", target: "country"},
		},
	},
	"/basicOpen/platformAuth/vcSeller/pageList": {
		storeType: "VC",
		fields: []storeField{
			{source: "vc_store_id", target: "sid"},
			{source: "name", target: "store_name"},
			// VC 接口只返回 region/region_name，不返回 country；
			// 只能用 region 填国家列，勿改成 country（会触发 missing required field）。
			{source: "region", target: "country"},
		},
	},
}

// storeDefaults 补充 ls_stores 中 NOT NULL 列的默认值。
// 不同店铺接口返回字段不同：SC 返回 has_ads_setting，VC 不返回。
// ls_stores.has_ads_setting 是 NOT NULL，缺失时 UpsertRows 会写 NULL 导致 MySQL 报错，
// 因此在这里按接口补默认值。
var storeDefaults = map[string]map[string]any{
	"/basicOpen/platformAuth/vcSeller/pageList": {
		"has_ads_setting": int64(0),
	},
}

func normalizeStoreRows(path string, rows []map[string]any) error {
	mapping, ok := storeMappings[path]
	if !ok {
		return nil
	}
	defaults := storeDefaults[path]
	for i, row := range rows {
		for _, field := range mapping.fields {
			value, exists := row[field.source]
			if !exists {
				return fmt.Errorf("path %s row %d missing required field %q", path, i, field.source)
			}
			row[field.target] = value
		}
		row["store_type"] = mapping.storeType
		for col, val := range defaults {
			if _, exists := row[col]; !exists {
				row[col] = val
			}
		}
	}
	return nil
}
