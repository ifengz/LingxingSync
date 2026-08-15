package reportexport

import (
	"fmt"
)

const CustomerShipmentReplacementsReportType = "GET_FBA_FULFILLMENT_CUSTOMER_SHIPMENT_REPLACEMENT_DATA"

var replacementHeader = []string{
	"shipment-date", "sku", "asin", "fulfillment-center-id", "original-fulfillment-center-id", "quantity",
	"replacement-reason-code", "replacement-amazon-order-id", "original-amazon-order-id",
}

type CustomerShipmentReplacement struct {
	ShipmentDate                string
	SKU                         string
	ASIN                        string
	FulfillmentCenterID         string
	OriginalFulfillmentCenterID string
	Quantity                    int64
	QuantityRaw                 string
	ReplacementReasonCode       string
	ReplacementAmazonOrderID    string
	OriginalAmazonOrderID       string
}

func ParseCustomerShipmentReplacements(downloaded []byte, compressionAlgorithm, contentType string) ([]CustomerShipmentReplacement, error) {
	records, err := readExactTSV(downloaded, compressionAlgorithm, contentType, "replacement", replacementHeader)
	if err != nil {
		return nil, err
	}
	rows := make([]CustomerShipmentReplacement, 0, len(records))
	for i, record := range records {
		quantities, err := parseIntegerColumns("replacement", i+2, record, 5)
		if err != nil {
			return nil, err
		}
		rows = append(rows, CustomerShipmentReplacement{
			ShipmentDate: record[0], SKU: record[1], ASIN: record[2], FulfillmentCenterID: record[3],
			OriginalFulfillmentCenterID: record[4], Quantity: quantities[5], QuantityRaw: record[5],
			ReplacementReasonCode: record[6], ReplacementAmazonOrderID: record[7], OriginalAmazonOrderID: record[8],
		})
	}
	return rows, nil
}

func (r CustomerShipmentReplacement) Validate() error {
	if r.Quantity < 0 {
		return fmt.Errorf("replacement quantity cannot be negative")
	}
	return nil
}
