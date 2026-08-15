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
	ReservedInventoryReportType = "GET_RESERVED_INVENTORY_DATA"
	AFNInventoryReportType      = "GET_AFN_INVENTORY_DATA"
)

var fbaInventoryHeader = []string{
	"sku", "fnsku", "asin", "product-name", "condition", "your-price", "mfn-listing-exists", "mfn-fulfillable-quantity",
	"afn-listing-exists", "afn-warehouse-quantity", "afn-fulfillable-quantity", "afn-unsellable-quantity", "afn-reserved-quantity",
	"afn-total-quantity", "per-unit-volume", "afn-inbound-working-quantity", "afn-inbound-shipped-quantity",
	"afn-inbound-receiving-quantity", "afn-researching-quantity", "afn-reserved-future-supply", "afn-future-supply-buyable",
}

var reservedInventoryHeader = []string{
	"sku", "fnsku", "asin", "product-name", "reserved_qty", "reserved_customerorders", "reserved_fc-processing",
}

var afnInventoryHeader = []string{
	"seller-sku", "fulfillment-channel-sku", "asin", "condition-type", "Warehouse-Condition-code", "Quantity Available",
}

type FBAInventory struct {
	SKU                         string
	FNSKU                       string
	ASIN                        string
	ProductName                 string
	Condition                   string
	YourPrice                   string
	MFNListingExists            string
	MFNFulfillableQuantity      int64
	MFNFulfillableQuantityRaw   string
	AFNListingExists            string
	AFNWarehouseQuantity        int64
	AFNWarehouseQuantityRaw     string
	AFNFulfillableQuantity      int64
	AFNFulfillableQuantityRaw   string
	AFNUnsellableQuantity       int64
	AFNUnsellableQuantityRaw    string
	AFNReservedQuantity         int64
	AFNReservedQuantityRaw      string
	AFNTotalQuantity            int64
	AFNTotalQuantityRaw         string
	PerUnitVolume               string
	AFNInboundWorkingQuantity   int64
	AFNInboundWorkingRaw        string
	AFNInboundShippedQuantity   int64
	AFNInboundShippedRaw        string
	AFNInboundReceivingQuantity int64
	AFNInboundReceivingRaw      string
	AFNResearchingQuantity      int64
	AFNResearchingQuantityRaw   string
	AFNReservedFutureSupply     int64
	AFNReservedFutureSupplyRaw  string
	AFNFutureSupplyBuyable      string
}

type ReservedInventory struct {
	SKU                       string
	FNSKU                     string
	ASIN                      string
	ProductName               string
	ReservedQty               int64
	ReservedQtyRaw            string
	ReservedCustomerOrders    int64
	ReservedCustomerOrdersRaw string
	ReservedFCProcessing      int64
	ReservedFCProcessingRaw   string
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
	records, err := readExactTSV(downloaded, compressionAlgorithm, contentType, "FBA inventory", fbaInventoryHeader)
	if err != nil {
		return nil, err
	}
	rows := make([]FBAInventory, 0, len(records))
	for i, record := range records {
		quantities, err := parseIntegerColumns("FBA inventory", i+2, record, 7, 9, 10, 11, 12, 13, 15, 16, 17, 18, 19)
		if err != nil {
			return nil, err
		}
		rows = append(rows, FBAInventory{
			SKU: record[0], FNSKU: record[1], ASIN: record[2], ProductName: record[3], Condition: record[4], YourPrice: record[5], MFNListingExists: record[6],
			MFNFulfillableQuantity: quantities[7], MFNFulfillableQuantityRaw: record[7], AFNListingExists: record[8],
			AFNWarehouseQuantity: quantities[9], AFNWarehouseQuantityRaw: record[9], AFNFulfillableQuantity: quantities[10], AFNFulfillableQuantityRaw: record[10],
			AFNUnsellableQuantity: quantities[11], AFNUnsellableQuantityRaw: record[11], AFNReservedQuantity: quantities[12], AFNReservedQuantityRaw: record[12],
			AFNTotalQuantity: quantities[13], AFNTotalQuantityRaw: record[13], PerUnitVolume: record[14],
			AFNInboundWorkingQuantity: quantities[15], AFNInboundWorkingRaw: record[15], AFNInboundShippedQuantity: quantities[16], AFNInboundShippedRaw: record[16],
			AFNInboundReceivingQuantity: quantities[17], AFNInboundReceivingRaw: record[17], AFNResearchingQuantity: quantities[18], AFNResearchingQuantityRaw: record[18],
			AFNReservedFutureSupply: quantities[19], AFNReservedFutureSupplyRaw: record[19], AFNFutureSupplyBuyable: record[20],
		})
	}
	return rows, nil
}

func ParseReservedInventory(downloaded []byte, compressionAlgorithm, contentType string) ([]ReservedInventory, error) {
	records, err := readExactTSV(downloaded, compressionAlgorithm, contentType, "reserved inventory", reservedInventoryHeader)
	if err != nil {
		return nil, err
	}
	rows := make([]ReservedInventory, 0, len(records))
	for i, record := range records {
		quantities, err := parseIntegerColumns("reserved inventory", i+2, record, 4, 5, 6)
		if err != nil {
			return nil, err
		}
		rows = append(rows, ReservedInventory{
			SKU: record[0], FNSKU: record[1], ASIN: record[2], ProductName: record[3], ReservedQty: quantities[4], ReservedQtyRaw: record[4],
			ReservedCustomerOrders: quantities[5], ReservedCustomerOrdersRaw: record[5],
			ReservedFCProcessing: quantities[6], ReservedFCProcessingRaw: record[6],
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
	payload, err := decompress(downloaded, compressionAlgorithm)
	if err != nil {
		return nil, err
	}
	payload, err = decodeReportText(payload, contentType)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(bytes.NewReader(payload))
	reader.Comma = '\t'
	reader.FieldsPerRecord = len(header)
	actual, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read %s TSV header: %w", name, err)
	}
	if len(actual) != len(header) {
		return nil, fmt.Errorf("%s TSV header has %d columns, want %d", name, len(actual), len(header))
	}
	for i, want := range header {
		if actual[i] != want {
			return nil, fmt.Errorf("%s TSV header column %d = %q, want %q", name, i+1, actual[i], want)
		}
	}
	rows := make([][]string, 0)
	for line := 2; ; line++ {
		record, err := reader.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read %s TSV row %d: %w", name, line, err)
		}
		rows = append(rows, record)
	}
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

func recordHeader(name string, column int) string {
	switch name {
	case "FBA inventory":
		return fbaInventoryHeader[column]
	case "reserved inventory":
		return reservedInventoryHeader[column]
	default:
		return "quantity"
	}
}
