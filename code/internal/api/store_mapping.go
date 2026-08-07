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
			{source: "region", target: "country"},
		},
	},
	"/basicOpen/platformAuth/vcSeller/pageList": {
		storeType: "VC",
		fields: []storeField{
			{source: "vc_store_id", target: "sid"},
			{source: "name", target: "store_name"},
			{source: "region", target: "country"},
		},
	},
}

func normalizeStoreRows(path string, rows []map[string]any) error {
	mapping, ok := storeMappings[path]
	if !ok {
		return nil
	}
	for i, row := range rows {
		for _, field := range mapping.fields {
			value, exists := row[field.source]
			if !exists {
				return fmt.Errorf("path %s row %d missing required field %q", path, i, field.source)
			}
			row[field.target] = value
		}
		row["store_type"] = mapping.storeType
	}
	return nil
}
