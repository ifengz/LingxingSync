package reportexport

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

const CustomerShipmentSalesReportType = "GET_FBA_FULFILLMENT_CUSTOMER_SHIPMENT_SALES_DATA"

var customerShipmentSalesHeader = []string{
	"shipment-date", "sku", "fnsku", "asin", "fulfillment-center-id", "quantity", "amazon-order-id", "currency",
	"item-price-per-unit", "shipping-price", "gift-wrap-price", "ship-city", "ship-state", "ship-postal-code",
}

type CustomerShipmentSale struct {
	ShipmentDate        string
	SKU                 string
	FNSKU               string
	ASIN                string
	FulfillmentCenterID string
	Quantity            int
	QuantityRaw         string
	AmazonOrderID       string
	Currency            string
	ItemPricePerUnit    float64
	ItemPricePerUnitRaw string
	ShippingPrice       string
	GiftWrapPrice       string
	ShipCity            string
	ShipState           string
	ShipPostalCode      string
}

func ParseCustomerShipmentSales(downloaded []byte, compressionAlgorithm, contentType string) ([]CustomerShipmentSale, error) {
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
	reader.FieldsPerRecord = len(customerShipmentSalesHeader)
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read customer shipment sales TSV header: %w", err)
	}
	if len(header) != len(customerShipmentSalesHeader) {
		return nil, fmt.Errorf("customer shipment sales TSV header has %d columns, want %d", len(header), len(customerShipmentSalesHeader))
	}
	for i, want := range customerShipmentSalesHeader {
		if header[i] != want {
			return nil, fmt.Errorf("customer shipment sales TSV header column %d = %q, want %q", i+1, header[i], want)
		}
	}
	rows := make([]CustomerShipmentSale, 0)
	for line := 2; ; line++ {
		record, err := reader.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read customer shipment sales TSV row %d: %w", line, err)
		}
		quantity, err := strconv.Atoi(record[5])
		if err != nil {
			return nil, fmt.Errorf("customer shipment sales TSV row %d quantity %q: %w", line, record[5], err)
		}
		price, err := strconv.ParseFloat(record[8], 64)
		if err != nil {
			return nil, fmt.Errorf("customer shipment sales TSV row %d item-price-per-unit %q: %w", line, record[8], err)
		}
		rows = append(rows, CustomerShipmentSale{
			ShipmentDate: record[0], SKU: record[1], FNSKU: record[2], ASIN: record[3], FulfillmentCenterID: record[4],
			Quantity: quantity, QuantityRaw: record[5], AmazonOrderID: record[6], Currency: record[7],
			ItemPricePerUnit: price, ItemPricePerUnitRaw: record[8], ShippingPrice: record[9], GiftWrapPrice: record[10],
			ShipCity: record[11], ShipState: record[12], ShipPostalCode: record[13],
		})
	}
}
