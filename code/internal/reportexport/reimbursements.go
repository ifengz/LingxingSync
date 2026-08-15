package reportexport

const FBAReimbursementsReportType = "GET_FBA_REIMBURSEMENTS_DATA"

var reimbursementHeader = []string{
	"approval-date", "reimbursement-id", "case-id", "amazon-order-id", "reason", "sku", "fnsku", "asin", "product-name", "condition",
	"currency-unit", "amount-per-unit", "amount-total", "quantity-reimbursed-cash", "quantity-reimbursed-inventory", "quantity-reimbursed-total",
	"original-reimbursement-id", "original-reimbursement-type",
}

type FBAReimbursement struct {
	ApprovalDate                string
	ReimbursementID             string
	CaseID                      string
	AmazonOrderID               string
	Reason                      string
	SKU                         string
	FNSKU                       string
	ASIN                        string
	ProductName                 string
	Condition                   string
	CurrencyUnit                string
	AmountPerUnit               string
	AmountTotal                 string
	QuantityReimbursedCash      string
	QuantityReimbursedInventory string
	QuantityReimbursedTotal     string
	OriginalReimbursementID     string
	OriginalReimbursementType   string
}

func ParseFBAReimbursements(downloaded []byte, compressionAlgorithm, contentType string) ([]FBAReimbursement, error) {
	records, err := readExactTSV(downloaded, compressionAlgorithm, contentType, "reimbursements", reimbursementHeader)
	if err != nil {
		return nil, err
	}
	rows := make([]FBAReimbursement, 0, len(records))
	for _, record := range records {
		rows = append(rows, FBAReimbursement{
			ApprovalDate: record[0], ReimbursementID: record[1], CaseID: record[2], AmazonOrderID: record[3], Reason: record[4],
			SKU: record[5], FNSKU: record[6], ASIN: record[7], ProductName: record[8], Condition: record[9], CurrencyUnit: record[10],
			AmountPerUnit: record[11], AmountTotal: record[12], QuantityReimbursedCash: record[13], QuantityReimbursedInventory: record[14], QuantityReimbursedTotal: record[15],
			OriginalReimbursementID: record[16], OriginalReimbursementType: record[17],
		})
	}
	return rows, nil
}
