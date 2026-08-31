package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fixedDatasetRequestLogReader struct {
	query datasetRequestLogQuery
	page  datasetRequestLogPage
}

func (r *fixedDatasetRequestLogReader) List(_ context.Context, q datasetRequestLogQuery) (datasetRequestLogPage, error) {
	r.query = q
	return r.page, nil
}

func TestDatasetRequestsEndpointForwardsFiltersAndPagination(t *testing.T) {
	reader := &fixedDatasetRequestLogReader{page: datasetRequestLogPage{Items: []datasetRequestLogItem{{ID: 9, DatasetID: "listing-daily-v1", Endpoint: "snapshot", ProjectID: "polabel2", StatusCode: 200, RowsReturned: 12}}, Total: 1}}
	s := &Server{datasetRequestLog: reader}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dataset-requests?dataset=listing-daily-v1&endpoint=snapshot&project=polabel2&status=ok&date_from=2026-08-01&date_to=2026-08-25&page=2&page_size=10", nil)

	s.apiDatasetRequests(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"project_id":"polabel2"`) || !strings.Contains(rec.Body.String(), `"rows_returned":12`) || !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("dataset requests response status=%d body=%s", rec.Code, rec.Body.String())
	}
	if reader.query.Dataset != "listing-daily-v1" || reader.query.Endpoint != "snapshot" || reader.query.Project != "polabel2" || reader.query.Status != "ok" || reader.query.DateFrom != "2026-08-01" || reader.query.DateTo != "2026-08-25" || reader.query.Page != 2 || reader.query.PageSize != 10 {
		t.Fatalf("dataset requests query=%+v", reader.query)
	}
}
