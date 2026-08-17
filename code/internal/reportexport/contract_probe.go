package reportexport

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

const FBAInventoryPlanningReportType = "GET_FBA_INVENTORY_PLANNING_DATA"

// ContractProbeResult is the evidence needed to freeze a report contract
// before adding a parser or a raw table.
type ContractProbeResult struct {
	ReportTaskID         string
	ReportDocumentID     string
	CompressionAlgorithm string
	ContentType          string
	DownloadSHA256       string
	DownloadedBytes      int64
	Header               []string
	Rows                 int
}

// ProbeFBAInventoryPlanning creates one formal Inventory Planning report and
// returns only its transport metadata and TSV contract. It never connects to
// or writes the local database.
func ProbeFBAInventoryPlanning(ctx context.Context, client SignedJSONClient, request Request) (ContractProbeResult, error) {
	if client == nil {
		return ContractProbeResult{}, fmt.Errorf("report contract probe: client is required")
	}
	if normalizedReportType(request) != FBAInventoryPlanningReportType {
		return ContractProbeResult{}, fmt.Errorf("report contract probe: expected %s", FBAInventoryPlanningReportType)
	}
	if err := validateReportRequestFields(request); err != nil {
		return ContractProbeResult{}, err
	}

	runner := Runner{Client: client, Store: contractProbeStore{}}
	raw, err := runner.call(ctx, createPath, createBody(request))
	if err != nil {
		return ContractProbeResult{}, err
	}
	var created struct {
		TaskID string `json:"task_id"`
	}
	if err := decodeEnvelope(raw, &created); err != nil {
		return ContractProbeResult{}, err
	}
	if created.TaskID == "" {
		return ContractProbeResult{}, fmt.Errorf("report contract probe: create response missing data.task_id")
	}

	data, err := runner.waitForDone(ctx, request, 0, created.TaskID, "")
	if err != nil {
		return ContractProbeResult{ReportTaskID: created.TaskID}, err
	}
	if data.ReportDocumentID == "" || data.URL == "" {
		return ContractProbeResult{ReportTaskID: created.TaskID}, fmt.Errorf("report contract probe: DONE response missing report_document_id or url")
	}
	body, hash, contentType, err := runner.download(ctx, request, data)
	if err != nil {
		return ContractProbeResult{ReportTaskID: created.TaskID, ReportDocumentID: data.ReportDocumentID}, err
	}
	header, rows, err := readProbeTSV(body, data.CompressionAlgorithm, contentType)
	if err != nil {
		return ContractProbeResult{ReportTaskID: created.TaskID, ReportDocumentID: data.ReportDocumentID, DownloadSHA256: hash, DownloadedBytes: int64(len(body)), ContentType: contentType, CompressionAlgorithm: data.CompressionAlgorithm}, err
	}
	return ContractProbeResult{
		ReportTaskID:         created.TaskID,
		ReportDocumentID:     data.ReportDocumentID,
		CompressionAlgorithm: data.CompressionAlgorithm,
		ContentType:          contentType,
		DownloadSHA256:       hash,
		DownloadedBytes:      int64(len(body)),
		Header:               header,
		Rows:                 rows,
	}, nil
}

func readProbeTSV(downloaded []byte, compressionAlgorithm, contentType string) ([]string, int, error) {
	payload, err := decompress(downloaded, compressionAlgorithm)
	if err != nil {
		return nil, 0, err
	}
	payload, err = decodeReportText(payload, contentType)
	if err != nil {
		return nil, 0, err
	}
	reader := csv.NewReader(bytes.NewReader(payload))
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("report contract probe: read TSV header: %w", err)
	}
	if len(header) == 0 || strings.TrimSpace(strings.Join(header, "")) == "" {
		return nil, 0, fmt.Errorf("report contract probe: TSV header is empty")
	}
	rows := 0
	for {
		_, err := reader.Read()
		if err == io.EOF {
			return header, rows, nil
		}
		if err != nil {
			return nil, 0, fmt.Errorf("report contract probe: read TSV row %d: %w", rows+2, err)
		}
		rows++
	}
}

type contractProbeStore struct{}

func (contractProbeStore) EnsureReport(context.Context, Request) (Audit, error) { return Audit{}, nil }
func (contractProbeStore) LoadReport(context.Context, int64) (Audit, error)     { return Audit{}, nil }
func (contractProbeStore) MarkReportCreated(context.Context, int64, string) error {
	return nil
}
func (contractProbeStore) MarkReportProgress(context.Context, int64, string, string, string, string) error {
	return nil
}
func (contractProbeStore) SaveCustomerReturns(context.Context, int64, []CustomerReturn, string, string) error {
	return nil
}
func (contractProbeStore) MarkReportError(context.Context, int64, string, error) error { return nil }
