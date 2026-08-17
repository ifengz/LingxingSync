package datasetapi

import (
	"fmt"
	"sort"
	"strings"
)

type Column struct {
	Name     string
	SQLType  string
	Nullable bool
}

type Schema struct {
	DatasetID  string
	TableName  string
	DataNote   string
	Columns    []Column
	PrimaryKey []string
}

var schemas = map[string]Schema{
	"listing-daily-v1": {
		DatasetID: "listing-daily-v1", TableName: "listing_daily_v1",
		DataNote: "日维数据；同一业务日期按店铺、渠道、ASIN、SKU 保留一行。",
		Columns: append(schemaColumns([]Column{
			{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "channel", SQLType: "VARCHAR(32)", Nullable: false},
			{Name: "asin", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "sku", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "business_date", SQLType: "DATE", Nullable: false},
			{Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}, {Name: "is_provisional", SQLType: "TINYINT(1)", Nullable: false}, {Name: "verification_status", SQLType: "VARCHAR(32)", Nullable: false},
		}), listingBusinessColumns()...),
		PrimaryKey: []string{"store", "channel", "asin", "sku", "business_date"},
	},
	"return-reason-detail-v1": {
		DatasetID: "return-reason-detail-v1", TableName: "return_reason_detail_v1", DataNote: "退货原因明细；按退货记录稳定键增量读取。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"order-shipping-address-detail-v1": {
		DatasetID: "order-shipping-address-detail-v1", TableName: "order_shipping_address_detail_v1", DataNote: "订单配送地址行明细；原始接口只提供城市、州、省、邮编和国家等字段。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"fba-inventory-snapshot-v1": {
		DatasetID: "fba-inventory-snapshot-v1", TableName: "fba_inventory_snapshot_v1", DataNote: "FBA 库存每日快照；历史从本版本部署后每次成功同步开始累计，不补造部署前日期。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
}

func schemaColumns(columns []Column) []Column { return append([]Column(nil), columns...) }

func detailSchemaColumns(fixed []Column) []Column {
	return append([]Column(nil), fixed...)
}

func listingBusinessColumns() []Column {
	columns := make([]Column, 0, len(availableSchemaFields))
	for _, name := range availableSchemaFields {
		columns = append(columns, Column{Name: name, SQLType: schemaType(name), Nullable: true})
	}
	return columns
}

var availableSchemaFields = []string{
	"sales_units", "sales_amount", "returns_qty", "inventory_sellable", "inventory_inbound", "inventory_reserved", "inventory_unfulfillable", "inventory_local_warehouse", "inventory_unhealthy_units", "inventory_aged90_sellable_units", "inventory_sell_through_rate", "inventory_receive_fill_rate", "inventory_vendor_confirmation_rate", "inventory_avg_lead_time_days", "inventory_sellable_cost", "inventory_unfulfillable_cost", "inventory_aged90_cost", "inventory_unhealthy_cost", "inventory_inbound_cost", "inventory_currency", "inventory_inbound_receiving", "inventory_inbound_shipped", "inventory_inbound_working", "inventory_reserved_customer_orders", "inventory_reserved_fc_processing", "inventory_reserved_fc_transfers", "sessions_desktop", "sessions_mobile", "sessions_total", "review_count", "rating", "sp_spend", "sp_sales", "sp_orders", "sd_spend", "sd_sales", "sd_orders", "hsa_spend", "hsa_sales", "hsa_orders", "sb_spend", "sb_sales", "sb_orders",
}

func columnsForFields(fields []string) []Column {
	out := make([]Column, 0, len(fields))
	for _, name := range fields {
		out = append(out, Column{Name: name, SQLType: schemaType(name), Nullable: true})
	}
	return out
}

func schemaType(name string) string {
	switch {
	case strings.HasSuffix(name, "_units"), strings.HasSuffix(name, "_orders"), strings.HasSuffix(name, "_count"), strings.HasPrefix(name, "sessions_"), strings.HasPrefix(name, "inv_age_"), name == "quantity", name == "quantity_shipped", strings.HasSuffix(name, "_quantity"), name == "returns_qty":
		return "BIGINT"
	case strings.HasSuffix(name, "_date"), strings.HasSuffix(name, "_date_locale"):
		return "VARCHAR(64)"
	case name == "inventory_currency", name == "currency", strings.HasSuffix(name, "_status"):
		return "VARCHAR(255)"
	case strings.Contains(name, "rate"), name == "rating", name == "sell_through", name == "historical_days_of_supply", name == "inventory_avg_lead_time_days":
		return "DECIMAL(20,6)"
	case strings.Contains(name, "amount"), strings.Contains(name, "cost"), strings.Contains(name, "spend"), strings.HasSuffix(name, "_sales"):
		return "DECIMAL(20,6)"
	case strings.HasPrefix(name, "inventory_"):
		return "BIGINT"
	default:
		return "VARCHAR(255)"
	}
}

func SchemaFor(id string) (Schema, bool) {
	schema, ok := schemas[id]
	if !ok {
		return Schema{}, false
	}
	definition, _ := DefinitionFor(id)
	if id != "listing-daily-v1" {
		schema.Columns = append(fixedColumnsForDataset(), columnsForFields(definition.Fields)...)
	}
	schema.Columns = append([]Column(nil), schema.Columns...)
	schema.PrimaryKey = append([]string(nil), schema.PrimaryKey...)
	return schema, true
}

func (s Schema) CreateTableSQL(selected []string) (string, error) {
	allowed := make(map[string]Column, len(s.Columns))
	for _, column := range s.Columns {
		allowed[column.Name] = column
	}
	definition, _ := DefinitionFor(s.DatasetID)
	definitionFields := definition.Fields
	if s.DatasetID == DatasetID {
		definitionFields = availableSchemaFields
	}
	business := make(map[string]struct{}, len(definitionFields))
	for _, name := range definitionFields {
		business[name] = struct{}{}
	}
	wanted := make([]string, 0, len(s.Columns))
	for _, column := range s.Columns {
		if _, ok := business[column.Name]; !ok {
			wanted = append(wanted, column.Name)
		}
	}
	if len(selected) == 0 {
		for _, column := range s.Columns {
			if _, ok := business[column.Name]; ok {
				wanted = append(wanted, column.Name)
			}
		}
	} else {
		wanted = append(wanted, selected...)
	}
	seen := make(map[string]struct{}, len(wanted))
	columns := make([]Column, 0, len(wanted)+len(s.PrimaryKey))
	for _, name := range wanted {
		if _, ok := seen[name]; ok {
			continue
		}
		column, ok := allowed[name]
		if !ok {
			return "", fmt.Errorf("字段不可用于 %s: %s", s.DatasetID, name)
		}
		seen[name] = struct{}{}
		columns = append(columns, column)
	}
	for _, name := range s.PrimaryKey {
		if _, ok := seen[name]; !ok {
			return "", fmt.Errorf("建表字段缺少主键列: %s", name)
		}
	}
	sort.SliceStable(columns, func(i, j int) bool { return indexOf(wanted, columns[i].Name) < indexOf(wanted, columns[j].Name) })
	lines := make([]string, 0, len(columns)+2)
	for _, column := range columns {
		nullable := " NULL"
		if !column.Nullable {
			nullable = " NOT NULL"
		}
		lines = append(lines, "  `"+column.Name+"` "+column.SQLType+nullable)
	}
	keys := make([]string, 0, len(s.PrimaryKey))
	for _, key := range s.PrimaryKey {
		keys = append(keys, "`"+key+"`")
	}
	lines = append(lines, "  PRIMARY KEY ("+strings.Join(keys, ", ")+")", "  KEY `idx_updated_at` (`updated_at`)")
	return "CREATE TABLE `" + s.TableName + "` (\n" + strings.Join(lines, ",\n") + "\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;", nil
}

func fixedColumnsForDataset() []Column {
	return []Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}
}

func indexOf(values []string, value string) int {
	for i, item := range values {
		if item == value {
			return i
		}
	}
	return len(values)
}
