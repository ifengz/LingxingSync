package reportexport

import (
	"fmt"
	"strconv"
)

const AFNInventoryByCountryReportType = "GET_AFN_INVENTORY_DATA_BY_COUNTRY"

var afnInventoryByCountryHeader = []string{
	"seller-sku", "fulfillment-channel-sku", "asin", "condition-type", "country", "quantity-for-local-fulfillment",
}

type AFNInventoryByCountry struct {
	SellerSKU                      string
	FulfillmentChannelSKU          string
	ASIN                           string
	ConditionType                  string
	Country                        string
	QuantityForLocalFulfillment    int64
	QuantityForLocalFulfillmentRaw string
}

func ParseAFNInventoryByCountry(downloaded []byte, compressionAlgorithm, contentType string) ([]AFNInventoryByCountry, error) {
	records, err := readExactTSV(downloaded, compressionAlgorithm, contentType, "AFN inventory by country", afnInventoryByCountryHeader)
	if err != nil {
		return nil, err
	}
	rows := make([]AFNInventoryByCountry, 0, len(records))
	for i, record := range records {
		quantity, err := strconv.ParseInt(record[5], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("AFN inventory by country TSV row %d quantity %q: %w", i+2, record[5], err)
		}
		rows = append(rows, AFNInventoryByCountry{
			SellerSKU: record[0], FulfillmentChannelSKU: record[1], ASIN: record[2], ConditionType: record[3], Country: record[4],
			QuantityForLocalFulfillment: quantity, QuantityForLocalFulfillmentRaw: record[5],
		})
	}
	return rows, nil
}
