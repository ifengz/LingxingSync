// Package listingdaily builds and publishes the one allowed listing daily fact set.
package listingdaily

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ScopeListing   Scope = "listing"
	ScopeStore     Scope = "store"
	ScopeAllocated Scope = "allocated"
)

var ErrReportReconciliationFailed = errors.New("listing daily: report reconciliation failed; batch was not published")

type Scope string

// Key is the contractual grain. Store-scoped HSA has an empty ASIN and SKU.
type Key struct {
	Store        string
	Channel      string
	ASIN         string
	SKU          string
	BusinessDate time.Time
}

type Values struct {
	SalesUnits                      *int64
	SalesAmount                     *float64
	ReturnsQty                      *int64
	InventorySellable               *int64
	InventoryInbound                *int64
	InventoryReserved               *int64
	InventoryUnfulfillable          *int64
	InventoryLocalWarehouse         *int64
	InventoryUnhealthyUnits         *int64
	InventoryAged90SellableUnits    *int64
	InventorySellThroughRate        *float64
	InventoryReceiveFillRate        *float64
	InventoryVendorConfirmationRate *float64
	InventoryAvgLeadTimeDays        *float64
	InventorySellableCost           *float64
	InventoryUnfulfillableCost      *float64
	InventoryAged90Cost             *float64
	InventoryUnhealthyCost          *float64
	InventoryInboundCost            *float64
	InventoryCurrency               *string
	InventoryInboundReceiving       *int64
	InventoryInboundShipped         *int64
	InventoryInboundWorking         *int64
	InventoryReservedCustomerOrders *int64
	InventoryReservedFCProcessing   *int64
	InventoryReservedFCTransfers    *int64
	SessionsDesktop                 *int64
	SessionsMobile                  *int64
	SessionsTotal                   *int64
	ReviewCount                     *int64
	Rating                          *float64
	SPSpend                         *float64
	SPSales                         *float64
	SPOrders                        *int64
	SDSpend                         *float64
	SDSales                         *float64
	SDOrders                        *int64
	HSASpend                        *float64
	HSASales                        *float64
	HSAOrders                       *int64
	SBSpend                         *float64
	SBSales                         *float64
	SBOrders                        *int64
}

type Source string

const (
	SourceAPI    Source = "api"
	SourceReport Source = "report"
)

type Input struct {
	Key    Key
	Scope  Scope
	Values Values
}

type Metric struct {
	Key            Key
	Scope          Scope
	Values         Values
	Sources        map[string]Source
	IsProvisional  bool
	IsVerified     bool
	VerifiedFields map[string]bool
}

type ReportState string

const (
	ReportAbsent     ReportState = "absent"
	ReportReconciled ReportState = "reconciled"
	ReportFailed     ReportState = "failed"
)

type Batch struct {
	Rows        []Metric
	ReportState ReportState
}

type Store interface {
	Persist(context.Context, []Metric) error
}

// Prepare validates and assigns publication metadata without writing. It lets
// callers collect several targets before one atomic Store.Persist call.
func Prepare(batch Batch, today time.Time) ([]Metric, error) {
	if batch.ReportState == ReportFailed {
		return nil, ErrReportReconciliationFailed
	}
	if batch.ReportState != "" && batch.ReportState != ReportAbsent && batch.ReportState != ReportReconciled {
		return nil, fmt.Errorf("listing daily: unsupported report state %q", batch.ReportState)
	}
	verified := batch.ReportState == ReportReconciled
	today = normalizedKey(Key{BusinessDate: today}).BusinessDate
	rows := make([]Metric, len(batch.Rows))
	for i, row := range batch.Rows {
		if err := validateInput(Input{Key: row.Key, Scope: row.Scope, Values: row.Values}); err != nil {
			return nil, err
		}
		row.Key = normalizedKey(row.Key)
		row.IsVerified = allFieldsVerified(row)
		row.IsProvisional = row.Key.BusinessDate.Equal(today) && !row.IsVerified
		if row.Sources == nil {
			row.Sources = make(map[string]Source)
			fallback := SourceAPI
			if verified {
				fallback = SourceReport
			}
			for _, field := range knownFields(row.Values) {
				row.Sources[field] = fallback
			}
		}
		rows[i] = row
	}
	return rows, nil
}

// Assemble merges only normalized source inputs. Report fields replace API fields
// only after reconciliation; nil stays nil and is never converted to zero.
func Assemble(api, report []Input, reportReconciled bool) ([]Metric, error) {
	rows := make(map[string]Metric, len(api)+len(report))
	if err := apply(rows, api, false, SourceAPI); err != nil {
		return nil, err
	}
	if reportReconciled {
		if err := apply(rows, report, true, SourceReport); err != nil {
			return nil, err
		}
	}
	result := make([]Metric, 0, len(rows))
	for _, row := range rows {
		result = append(result, row)
	}
	return result, nil
}

func apply(rows map[string]Metric, inputs []Input, overwrite bool, source Source) error {
	for _, input := range inputs {
		if err := validateInput(input); err != nil {
			return err
		}
		id := keyID(input.Key, input.Scope)
		row, exists := rows[id]
		if !exists {
			row = Metric{Key: normalizedKey(input.Key), Scope: input.Scope}
		}
		row.Values = mergeValues(row.Values, input.Values, overwrite || !exists)
		row.Sources = mergeSources(row.Sources, input.Values, source)
		if row.VerifiedFields == nil {
			row.VerifiedFields = make(map[string]bool)
		}
		if source == SourceReport {
			for _, field := range knownFields(input.Values) {
				row.VerifiedFields[field] = true
			}
		}
		rows[id] = row
	}
	return nil
}

func mergeSources(base map[string]Source, incoming Values, source Source) map[string]Source {
	if base == nil {
		base = make(map[string]Source)
	}
	for _, field := range knownFields(incoming) {
		base[field] = source
	}
	return base
}

func knownFields(values Values) []string {
	fields := make([]string, 0, 15)
	if values.SalesUnits != nil {
		fields = append(fields, "sales_units")
	}
	if values.SalesAmount != nil {
		fields = append(fields, "sales_amount")
	}
	if values.ReturnsQty != nil {
		fields = append(fields, "returns_qty")
	}
	if values.InventorySellable != nil {
		fields = append(fields, "inventory_sellable")
	}
	if values.InventoryInbound != nil {
		fields = append(fields, "inventory_inbound")
	}
	if values.InventoryReserved != nil {
		fields = append(fields, "inventory_reserved")
	}
	if values.InventoryUnfulfillable != nil {
		fields = append(fields, "inventory_unfulfillable")
	}
	if values.InventoryLocalWarehouse != nil {
		fields = append(fields, "inventory_local_warehouse")
	}
	if values.InventoryUnhealthyUnits != nil {
		fields = append(fields, "inventory_unhealthy_units")
	}
	if values.InventoryAged90SellableUnits != nil {
		fields = append(fields, "inventory_aged90_sellable_units")
	}
	if values.InventorySellThroughRate != nil {
		fields = append(fields, "inventory_sell_through_rate")
	}
	if values.InventoryReceiveFillRate != nil {
		fields = append(fields, "inventory_receive_fill_rate")
	}
	if values.InventoryVendorConfirmationRate != nil {
		fields = append(fields, "inventory_vendor_confirmation_rate")
	}
	if values.InventoryAvgLeadTimeDays != nil {
		fields = append(fields, "inventory_avg_lead_time_days")
	}
	if values.InventorySellableCost != nil {
		fields = append(fields, "inventory_sellable_cost")
	}
	if values.InventoryUnfulfillableCost != nil {
		fields = append(fields, "inventory_unfulfillable_cost")
	}
	if values.InventoryAged90Cost != nil {
		fields = append(fields, "inventory_aged90_cost")
	}
	if values.InventoryUnhealthyCost != nil {
		fields = append(fields, "inventory_unhealthy_cost")
	}
	if values.InventoryInboundCost != nil {
		fields = append(fields, "inventory_inbound_cost")
	}
	if values.InventoryCurrency != nil {
		fields = append(fields, "inventory_currency")
	}
	if values.InventoryInboundReceiving != nil {
		fields = append(fields, "inventory_inbound_receiving")
	}
	if values.InventoryInboundShipped != nil {
		fields = append(fields, "inventory_inbound_shipped")
	}
	if values.InventoryInboundWorking != nil {
		fields = append(fields, "inventory_inbound_working")
	}
	if values.InventoryReservedCustomerOrders != nil {
		fields = append(fields, "inventory_reserved_customer_orders")
	}
	if values.InventoryReservedFCProcessing != nil {
		fields = append(fields, "inventory_reserved_fc_processing")
	}
	if values.InventoryReservedFCTransfers != nil {
		fields = append(fields, "inventory_reserved_fc_transfers")
	}
	if values.SessionsDesktop != nil {
		fields = append(fields, "sessions_desktop")
	}
	if values.SessionsMobile != nil {
		fields = append(fields, "sessions_mobile")
	}
	if values.SessionsTotal != nil {
		fields = append(fields, "sessions_total")
	}
	if values.ReviewCount != nil {
		fields = append(fields, "review_count")
	}
	if values.Rating != nil {
		fields = append(fields, "rating")
	}
	if values.SPSpend != nil {
		fields = append(fields, "sp_spend")
	}
	if values.SPSales != nil {
		fields = append(fields, "sp_sales")
	}
	if values.SPOrders != nil {
		fields = append(fields, "sp_orders")
	}
	if values.SDSpend != nil {
		fields = append(fields, "sd_spend")
	}
	if values.SDSales != nil {
		fields = append(fields, "sd_sales")
	}
	if values.SDOrders != nil {
		fields = append(fields, "sd_orders")
	}
	if values.HSASpend != nil {
		fields = append(fields, "hsa_spend")
	}
	if values.HSASales != nil {
		fields = append(fields, "hsa_sales")
	}
	if values.HSAOrders != nil {
		fields = append(fields, "hsa_orders")
	}
	if values.SBSpend != nil {
		fields = append(fields, "sb_spend")
	}
	if values.SBSales != nil {
		fields = append(fields, "sb_sales")
	}
	if values.SBOrders != nil {
		fields = append(fields, "sb_orders")
	}
	return fields
}

func validateInput(input Input) error {
	key := normalizedKey(input.Key)
	if key.Store == "" || key.Channel == "" || key.BusinessDate.IsZero() {
		return fmt.Errorf("listing daily: store, channel, and business date are required")
	}
	switch input.Scope {
	case ScopeListing:
		if key.ASIN == "" || key.SKU == "" {
			return fmt.Errorf("listing daily: listing scope requires ASIN and SKU")
		}
	case ScopeStore:
		if (key.Channel != "hsa" && key.Channel != "sb") || key.ASIN != "" || key.SKU != "" {
			return fmt.Errorf("listing daily: store scope is HSA/SB-only and cannot contain ASIN/SKU")
		}
	case ScopeAllocated:
		if (key.Channel != "hsa" && key.Channel != "sb") || key.ASIN == "" || key.SKU == "" {
			return fmt.Errorf("listing daily: allocated scope requires HSA/SB plus ASIN and SKU")
		}
	default:
		return fmt.Errorf("listing daily: unsupported scope %q", input.Scope)
	}
	return nil
}

func normalizedKey(key Key) Key {
	key.Store = strings.TrimSpace(key.Store)
	key.Channel = strings.TrimSpace(strings.ToLower(key.Channel))
	key.ASIN = strings.TrimSpace(key.ASIN)
	key.SKU = strings.TrimSpace(key.SKU)
	year, month, day := key.BusinessDate.Date()
	key.BusinessDate = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return key
}

func keyID(key Key, scope Scope) string {
	key = normalizedKey(key)
	return strings.Join([]string{key.Store, key.Channel, string(scope), key.ASIN, key.SKU, key.BusinessDate.Format("2006-01-02")}, "\x00")
}

func mergeValues(base, incoming Values, overwrite bool) Values {
	return Values{
		SalesUnits:                      choose(base.SalesUnits, incoming.SalesUnits, overwrite),
		SalesAmount:                     choose(base.SalesAmount, incoming.SalesAmount, overwrite),
		ReturnsQty:                      choose(base.ReturnsQty, incoming.ReturnsQty, overwrite),
		InventorySellable:               choose(base.InventorySellable, incoming.InventorySellable, overwrite),
		InventoryInbound:                choose(base.InventoryInbound, incoming.InventoryInbound, overwrite),
		InventoryReserved:               choose(base.InventoryReserved, incoming.InventoryReserved, overwrite),
		InventoryUnfulfillable:          choose(base.InventoryUnfulfillable, incoming.InventoryUnfulfillable, overwrite),
		InventoryLocalWarehouse:         choose(base.InventoryLocalWarehouse, incoming.InventoryLocalWarehouse, overwrite),
		InventoryUnhealthyUnits:         choose(base.InventoryUnhealthyUnits, incoming.InventoryUnhealthyUnits, overwrite),
		InventoryAged90SellableUnits:    choose(base.InventoryAged90SellableUnits, incoming.InventoryAged90SellableUnits, overwrite),
		InventorySellThroughRate:        choose(base.InventorySellThroughRate, incoming.InventorySellThroughRate, overwrite),
		InventoryReceiveFillRate:        choose(base.InventoryReceiveFillRate, incoming.InventoryReceiveFillRate, overwrite),
		InventoryVendorConfirmationRate: choose(base.InventoryVendorConfirmationRate, incoming.InventoryVendorConfirmationRate, overwrite),
		InventoryAvgLeadTimeDays:        choose(base.InventoryAvgLeadTimeDays, incoming.InventoryAvgLeadTimeDays, overwrite),
		InventorySellableCost:           choose(base.InventorySellableCost, incoming.InventorySellableCost, overwrite),
		InventoryUnfulfillableCost:      choose(base.InventoryUnfulfillableCost, incoming.InventoryUnfulfillableCost, overwrite),
		InventoryAged90Cost:             choose(base.InventoryAged90Cost, incoming.InventoryAged90Cost, overwrite),
		InventoryUnhealthyCost:          choose(base.InventoryUnhealthyCost, incoming.InventoryUnhealthyCost, overwrite),
		InventoryInboundCost:            choose(base.InventoryInboundCost, incoming.InventoryInboundCost, overwrite),
		InventoryCurrency:               choose(base.InventoryCurrency, incoming.InventoryCurrency, overwrite),
		InventoryInboundReceiving:       choose(base.InventoryInboundReceiving, incoming.InventoryInboundReceiving, overwrite),
		InventoryInboundShipped:         choose(base.InventoryInboundShipped, incoming.InventoryInboundShipped, overwrite),
		InventoryInboundWorking:         choose(base.InventoryInboundWorking, incoming.InventoryInboundWorking, overwrite),
		InventoryReservedCustomerOrders: choose(base.InventoryReservedCustomerOrders, incoming.InventoryReservedCustomerOrders, overwrite),
		InventoryReservedFCProcessing:   choose(base.InventoryReservedFCProcessing, incoming.InventoryReservedFCProcessing, overwrite),
		InventoryReservedFCTransfers:    choose(base.InventoryReservedFCTransfers, incoming.InventoryReservedFCTransfers, overwrite),
		SessionsDesktop:                 choose(base.SessionsDesktop, incoming.SessionsDesktop, overwrite),
		SessionsMobile:                  choose(base.SessionsMobile, incoming.SessionsMobile, overwrite),
		SessionsTotal:                   choose(base.SessionsTotal, incoming.SessionsTotal, overwrite),
		ReviewCount:                     choose(base.ReviewCount, incoming.ReviewCount, overwrite),
		Rating:                          choose(base.Rating, incoming.Rating, overwrite),
		SPSpend:                         choose(base.SPSpend, incoming.SPSpend, overwrite),
		SPSales:                         choose(base.SPSales, incoming.SPSales, overwrite),
		SPOrders:                        choose(base.SPOrders, incoming.SPOrders, overwrite),
		SDSpend:                         choose(base.SDSpend, incoming.SDSpend, overwrite),
		SDSales:                         choose(base.SDSales, incoming.SDSales, overwrite),
		SDOrders:                        choose(base.SDOrders, incoming.SDOrders, overwrite),
		HSASpend:                        choose(base.HSASpend, incoming.HSASpend, overwrite),
		HSASales:                        choose(base.HSASales, incoming.HSASales, overwrite),
		HSAOrders:                       choose(base.HSAOrders, incoming.HSAOrders, overwrite),
		SBSpend:                         choose(base.SBSpend, incoming.SBSpend, overwrite),
		SBSales:                         choose(base.SBSales, incoming.SBSales, overwrite),
		SBOrders:                        choose(base.SBOrders, incoming.SBOrders, overwrite),
	}
}

func choose[T any](base, incoming *T, overwrite bool) *T {
	if incoming != nil && (overwrite || base == nil) {
		return incoming
	}
	return base
}

// Publish refuses an incomplete report batch and assigns provisional status from
// the supplied business date rather than filling unknown metric values.
func Publish(ctx context.Context, store Store, batch Batch, today time.Time) error {
	if store == nil {
		return fmt.Errorf("listing daily: nil store")
	}
	rows, err := Prepare(batch, today)
	if err != nil {
		return err
	}
	return store.Persist(ctx, rows)
}

func allFieldsVerified(row Metric) bool {
	fields := knownFields(row.Values)
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		if !row.VerifiedFields[field] {
			return false
		}
	}
	return true
}
