package reportexport

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

const (
	FBAInventoryReportType      = "GET_FBA_MYI_UNSUPPRESSED_INVENTORY_DATA"
	FBAAllInventoryReportType   = "GET_FBA_MYI_ALL_INVENTORY_DATA"
	ReservedInventoryReportType = "GET_RESERVED_INVENTORY_DATA"
	AFNInventoryReportType      = "GET_AFN_INVENTORY_DATA"
)

var fbaInventoryHeader = []string{
	"sku", "fnsku", "asin", "product-name", "condition", "your-price", "mfn-listing-exists", "mfn-fulfillable-quantity",
	"afn-listing-exists", "afn-warehouse-quantity", "afn-fulfillable-quantity", "afn-unsellable-quantity", "afn-reserved-quantity",
	"afn-total-quantity", "per-unit-volume", "afn-inbound-working-quantity", "afn-inbound-shipped-quantity",
	"afn-inbound-receiving-quantity", "afn-researching-quantity", "afn-reserved-future-supply", "afn-future-supply-buyable",
}

var fbaInventoryEUHeader = append(append([]string(nil), fbaInventoryHeader...), "afn-fulfillable-quantity-local", "afn-fulfillable-quantity-remote")

var fbaInventoryProductionHeader = append(append([]string(nil), fbaInventoryHeader...), "afn-fc-transfer-quantity", "afn-onhand-buyable-quantity", "store")

var reservedInventoryHeader = []string{
	"sku", "fnsku", "asin", "product-name", "reserved_qty", "reserved_customerorders", "reserved_fc-transfers", "reserved_fc-processing",
}

var reservedInventoryProductionHeader = []string{
	"sku", "fnsku", "asin", "product-name", "reserved_qty", "reserved_customerorders", "reserved_fc-processing", "reserved_staging", "program",
}

var afnInventoryHeader = []string{
	"seller-sku", "fulfillment-channel-sku", "asin", "condition-type", "Warehouse-Condition-code", "Quantity Available",
}

type FBAInventory struct {
	SKU                          string
	FNSKU                        string
	ASIN                         string
	ProductName                  string
	Condition                    string
	YourPrice                    string
	MFNListingExists             string
	MFNFulfillableQuantity       *int64
	MFNFulfillableQuantityRaw    string
	AFNListingExists             string
	AFNWarehouseQuantity         int64
	AFNWarehouseQuantityRaw      string
	AFNFulfillableQuantity       int64
	AFNFulfillableQuantityRaw    string
	AFNUnsellableQuantity        int64
	AFNUnsellableQuantityRaw     string
	AFNReservedQuantity          int64
	AFNReservedQuantityRaw       string
	AFNTotalQuantity             int64
	AFNTotalQuantityRaw          string
	PerUnitVolume                string
	AFNInboundWorkingQuantity    int64
	AFNInboundWorkingRaw         string
	AFNInboundShippedQuantity    int64
	AFNInboundShippedRaw         string
	AFNInboundReceivingQuantity  int64
	AFNInboundReceivingRaw       string
	AFNResearchingQuantity       int64
	AFNResearchingQuantityRaw    string
	AFNReservedFutureSupply      int64
	AFNReservedFutureSupplyRaw   string
	AFNFutureSupplyBuyable       string
	AFNFulfillableQuantityLocal  string
	AFNFulfillableQuantityRemote string
	AFNFCTransferQuantity        string
	AFNOnhandBuyableQuantity     string
	Store                        string
}

// FBAAllInventory has the same official 21-column contract as the active MYI
// report, but is kept as a distinct type because archived inventory is a
// separate report evidence/table.
type FBAAllInventory FBAInventory

type ReservedInventory struct {
	SKU                       string
	FNSKU                     string
	ASIN                      string
	ProductName               string
	ReservedQty               int64
	ReservedQtyRaw            string
	ReservedCustomerOrders    int64
	ReservedCustomerOrdersRaw string
	ReservedFCTransfers       int64
	ReservedFCTransfersRaw    string
	ReservedFCProcessing      int64
	ReservedFCProcessingRaw   string
	ReservedStaging           int64
	ReservedStagingRaw        string
	Program                   string
}

type AFNInventory struct {
	SellerSKU              string
	FulfillmentChannelSKU  string
	ASIN                   string
	ConditionType          string
	WarehouseConditionCode string
	QuantityAvailable      int64
	QuantityAvailableRaw   string
}

func ParseFBAInventory(downloaded []byte, compressionAlgorithm, contentType string) ([]FBAInventory, error) {
	records, header, err := readExactTSVVariants(downloaded, compressionAlgorithm, contentType, "FBA inventory", [][]string{fbaInventoryHeader, fbaInventoryEUHeader, fbaInventoryProductionHeader})
	if err != nil {
		return nil, err
	}
	rows := make([]FBAInventory, 0, len(records))
	for i, record := range records {
		requiredQuantities, err := parseIntegerColumns("FBA inventory", i+2, record, 9, 10, 11, 12, 13, 15, 16, 17, 18, 19)
		if err != nil {
			return nil, err
		}
		mfnQuantity, err := parseOptionalIntegerColumn("FBA inventory", i+2, record, 7)
		if err != nil {
			return nil, err
		}
		row := FBAInventory{
			SKU: record[0], FNSKU: record[1], ASIN: record[2], ProductName: record[3], Condition: record[4], YourPrice: record[5], MFNListingExists: record[6],
			MFNFulfillableQuantity: mfnQuantity, MFNFulfillableQuantityRaw: record[7], AFNListingExists: record[8],
			AFNWarehouseQuantity: requiredQuantities[9], AFNWarehouseQuantityRaw: record[9], AFNFulfillableQuantity: requiredQuantities[10], AFNFulfillableQuantityRaw: record[10],
			AFNUnsellableQuantity: requiredQuantities[11], AFNUnsellableQuantityRaw: record[11], AFNReservedQuantity: requiredQuantities[12], AFNReservedQuantityRaw: record[12],
			AFNTotalQuantity: requiredQuantities[13], AFNTotalQuantityRaw: record[13], PerUnitVolume: record[14],
			AFNInboundWorkingQuantity: requiredQuantities[15], AFNInboundWorkingRaw: record[15], AFNInboundShippedQuantity: requiredQuantities[16], AFNInboundShippedRaw: record[16],
			AFNInboundReceivingQuantity: requiredQuantities[17], AFNInboundReceivingRaw: record[17], AFNResearchingQuantity: requiredQuantities[18], AFNResearchingQuantityRaw: record[18],
			AFNReservedFutureSupply: requiredQuantities[19], AFNReservedFutureSupplyRaw: record[19], AFNFutureSupplyBuyable: record[20],
		}
		switch len(header) {
		case len(fbaInventoryEUHeader):
			row.AFNFulfillableQuantityLocal = record[21]
			row.AFNFulfillableQuantityRemote = record[22]
		case len(fbaInventoryProductionHeader):
			row.AFNFCTransferQuantity = record[21]
			row.AFNOnhandBuyableQuantity = record[22]
			row.Store = record[23]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func ParseFBAAllInventory(downloaded []byte, compressionAlgorithm, contentType string) ([]FBAAllInventory, error) {
	rows, err := ParseFBAInventory(downloaded, compressionAlgorithm, contentType)
	if err != nil {
		return nil, err
	}
	result := make([]FBAAllInventory, len(rows))
	for i, row := range rows {
		result[i] = FBAAllInventory(row)
	}
	return result, nil
}

func ParseReservedInventory(downloaded []byte, compressionAlgorithm, contentType string) ([]ReservedInventory, error) {
	records, header, err := readExactTSVVariants(downloaded, compressionAlgorithm, contentType, "reserved inventory", [][]string{reservedInventoryHeader, reservedInventoryProductionHeader})
	if err != nil {
		return nil, err
	}
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	rows := make([]ReservedInventory, 0, len(records))
	for i, record := range records {
		quantityColumns := []int{index["reserved_qty"], index["reserved_customerorders"], index["reserved_fc-processing"]}
		if transferIndex, ok := index["reserved_fc-transfers"]; ok {
			quantityColumns = append(quantityColumns, transferIndex)
		}
		if stagingIndex, ok := index["reserved_staging"]; ok {
			quantityColumns = append(quantityColumns, stagingIndex)
		}
		quantities, err := parseIntegerColumns("reserved inventory", i+2, record, quantityColumns...)
		if err != nil {
			return nil, err
		}
		reservedQty := quantities[index["reserved_qty"]]
		reservedCustomerOrders := quantities[index["reserved_customerorders"]]
		reservedProcessing := quantities[index["reserved_fc-processing"]]
		reservedTransfers, reservedTransfersRaw := int64(0), ""
		if transferIndex, ok := index["reserved_fc-transfers"]; ok {
			reservedTransfers = quantities[transferIndex]
			reservedTransfersRaw = record[transferIndex]
		}
		reservedStaging, reservedStagingRaw := int64(0), ""
		if stagingIndex, ok := index["reserved_staging"]; ok {
			reservedStaging = quantities[stagingIndex]
			reservedStagingRaw = record[stagingIndex]
		}
		program := ""
		if programIndex, ok := index["program"]; ok {
			program = record[programIndex]
		}
		rows = append(rows, ReservedInventory{
			SKU: record[0], FNSKU: record[1], ASIN: record[2], ProductName: record[3], ReservedQty: reservedQty, ReservedQtyRaw: record[index["reserved_qty"]],
			ReservedCustomerOrders: reservedCustomerOrders, ReservedCustomerOrdersRaw: record[index["reserved_customerorders"]],
			ReservedFCTransfers: reservedTransfers, ReservedFCTransfersRaw: reservedTransfersRaw,
			ReservedFCProcessing: reservedProcessing, ReservedFCProcessingRaw: record[index["reserved_fc-processing"]],
			ReservedStaging: reservedStaging, ReservedStagingRaw: reservedStagingRaw,
			Program: program,
		})
	}
	return rows, nil
}

func ParseAFNInventory(downloaded []byte, compressionAlgorithm, contentType string) ([]AFNInventory, error) {
	records, err := readExactTSV(downloaded, compressionAlgorithm, contentType, "AFN inventory", afnInventoryHeader)
	if err != nil {
		return nil, err
	}
	rows := make([]AFNInventory, 0, len(records))
	for i, record := range records {
		quantity, err := strconv.ParseInt(record[5], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("AFN inventory TSV row %d quantity available %q: %w", i+2, record[5], err)
		}
		rows = append(rows, AFNInventory{SellerSKU: record[0], FulfillmentChannelSKU: record[1], ASIN: record[2], ConditionType: record[3], WarehouseConditionCode: record[4], QuantityAvailable: quantity, QuantityAvailableRaw: record[5]})
	}
	return rows, nil
}

func readExactTSV(downloaded []byte, compressionAlgorithm, contentType, name string, header []string) ([][]string, error) {
	records, _, err := readExactTSVVariants(downloaded, compressionAlgorithm, contentType, name, [][]string{header})
	return records, err
}

func readExactTSVVariants(downloaded []byte, compressionAlgorithm, contentType, name string, headers [][]string) ([][]string, []string, error) {
	payload, err := decompress(downloaded, compressionAlgorithm)
	if err != nil {
		return nil, nil, err
	}
	payload, err = decodeReportText(payload, contentType)
	if err != nil {
		return nil, nil, err
	}
	reader := csv.NewReader(bytes.NewReader(payload))
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	actual, err := reader.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("read %s TSV header: %w", name, err)
	}
	var matched []string
	var sameWidthMismatch error
	for _, candidate := range headers {
		candidateActual := trimTrailingEmptyField(actual, len(candidate))
		if len(candidateActual) != len(candidate) {
			continue
		}
		matches := true
		for i, want := range candidate {
			if candidateActual[i] != want {
				if sameWidthMismatch == nil {
					sameWidthMismatch = fmt.Errorf("%s TSV header column %d = %q, want %q", name, i+1, candidateActual[i], want)
				}
				matches = false
				break
			}
		}
		if matches {
			matched = candidate
			actual = candidateActual
			break
		}
	}
	if matched == nil {
		if sameWidthMismatch != nil {
			return nil, nil, fmt.Errorf("%w; actual header=%q", sameWidthMismatch, actual)
		}
		return nil, nil, fmt.Errorf("%s TSV header has %d columns, want one of %d or %d; actual header=%q", name, len(actual), len(headers[0]), len(headers[len(headers)-1]), actual)
	}
	rows := make([][]string, 0)
	for line := 2; ; line++ {
		record, err := reader.Read()
		if err == io.EOF {
			return rows, matched, nil
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read %s TSV row %d: %w", name, line, err)
		}
		record = trimTrailingEmptyField(record, len(matched))
		if len(record) != len(matched) {
			return nil, nil, fmt.Errorf("%s TSV row %d has %d columns, want %d", name, line, len(record), len(matched))
		}
		rows = append(rows, record)
	}
}

func optionalRecordValue(record []string, header []string, index int) string {
	if index >= len(header) || index >= len(record) {
		return ""
	}
	return record[index]
}

func trimTrailingEmptyField(record []string, expected int) []string {
	if len(record) == expected+1 && record[len(record)-1] == "" {
		return record[:expected]
	}
	return record
}

func parseIntegerColumns(name string, line int, record []string, columns ...int) (map[int]int64, error) {
	values := make(map[int]int64, len(columns))
	for _, column := range columns {
		value, err := strconv.ParseInt(record[column], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s TSV row %d column %q value %q: %w", name, line, recordHeader(name, column), record[column], err)
		}
		values[column] = value
	}
	return values, nil
}

func parseOptionalIntegerColumn(name string, line int, record []string, column int) (*int64, error) {
	if record[column] == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(record[column], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%s TSV row %d column %q value %q: %w", name, line, recordHeader(name, column), record[column], err)
	}
	return &value, nil
}

func recordHeader(name string, column int) string {
	switch name {
	case "FBA inventory":
		return fbaInventoryProductionHeader[column]
	case "reserved inventory":
		return reservedInventoryHeader[column]
	case "replacement":
		return replacementHeader[column]
	default:
		return "quantity"
	}
}
