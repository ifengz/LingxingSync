// Package reportexport implements the fixed OpenAPI evidence flow for Amazon
// reports. It intentionally does not share EndpointWorker's JSON pagination.
package reportexport

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

const CustomerReturnsReportType = "GET_FBA_FULFILLMENT_CUSTOMER_RETURNS_DATA"

var customerReturnsHeader = []string{
	"return-date", "order-id", "sku", "asin", "fnsku", "product-name", "quantity",
	"fulfillment-center-id", "detailed-disposition", "reason", "status", "license-plate-number", "customer-comments",
}

// CustomerReturn is one untouched formal TSV row, except quantity is checked
// as an integer before it is persisted.
type CustomerReturn struct {
	ReturnDate          string
	OrderID             string
	SKU                 string
	ASIN                string
	FNSKU               string
	ProductName         string
	Quantity            int
	QuantityRaw         string
	FulfillmentCenterID string
	DetailedDisposition string
	Reason              string
	Status              string
	LicensePlateNumber  string
	CustomerComments    string
}

// ParseCustomerReturns validates the complete report before any row reaches
// MySQL. Header aliases, missing fields and malformed quantities are errors.
func ParseCustomerReturns(downloaded []byte, compressionAlgorithm, contentType string) ([]CustomerReturn, error) {
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
	reader.FieldsPerRecord = len(customerReturnsHeader)
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read customer returns TSV header: %w", err)
	}
	if len(header) != len(customerReturnsHeader) {
		return nil, fmt.Errorf("customer returns TSV header has %d columns, want %d", len(header), len(customerReturnsHeader))
	}
	for i, want := range customerReturnsHeader {
		if header[i] != want {
			return nil, fmt.Errorf("customer returns TSV header column %d = %q, want %q", i+1, header[i], want)
		}
	}
	rows := make([]CustomerReturn, 0)
	for line := 2; ; line++ {
		record, err := reader.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read customer returns TSV row %d: %w", line, err)
		}
		quantity, err := strconv.Atoi(record[6])
		if err != nil {
			return nil, fmt.Errorf("customer returns TSV row %d quantity %q: %w", line, record[6], err)
		}
		rows = append(rows, CustomerReturn{
			ReturnDate: record[0], OrderID: record[1], SKU: record[2], ASIN: record[3], FNSKU: record[4], ProductName: record[5], Quantity: quantity, QuantityRaw: record[6],
			FulfillmentCenterID: record[7], DetailedDisposition: record[8], Reason: record[9], Status: record[10], LicensePlateNumber: record[11], CustomerComments: record[12],
		})
	}
}

func decodeReportText(payload []byte, contentType string) ([]byte, error) {
	charset := ""
	if strings.TrimSpace(contentType) != "" {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return nil, fmt.Errorf("customer returns report content type %q: %w", contentType, err)
		}
		charset = strings.ToLower(strings.TrimSpace(params["charset"]))
	}
	switch charset {
	case "", "utf-8", "utf8", "us-ascii":
		if !utf8.Valid(payload) {
			return nil, fmt.Errorf("customer returns TSV is not valid UTF-8")
		}
		return payload, nil
	case "cp1252", "windows-1252":
		decoded, err := charmap.Windows1252.NewDecoder().Bytes(payload)
		if err != nil {
			return nil, fmt.Errorf("decode customer returns TSV charset %q: %w", charset, err)
		}
		return decoded, nil
	default:
		return nil, fmt.Errorf("unsupported customer returns TSV charset %q", charset)
	}
}

func decompress(downloaded []byte, compressionAlgorithm string) ([]byte, error) {
	compression := strings.ToUpper(strings.TrimSpace(compressionAlgorithm))
	if compression != "" && compression != "GZIP" {
		return nil, fmt.Errorf("unsupported compression algorithm %q", compressionAlgorithm)
	}
	gzipExpected := compression == "GZIP"
	gzipMagic := len(downloaded) >= 2 && downloaded[0] == 0x1f && downloaded[1] == 0x8b
	if !gzipExpected && !gzipMagic {
		return downloaded, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(downloaded))
	if err != nil {
		return nil, fmt.Errorf("open customer returns GZIP: %w", err)
	}
	decompressed, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read customer returns GZIP: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close customer returns GZIP: %w", closeErr)
	}
	return decompressed, nil
}
