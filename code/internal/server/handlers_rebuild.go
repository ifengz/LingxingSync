package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/rebuild"
)

// rebuildStatus tracks the state of an async listing-daily rebuild.
type rebuildStatus struct {
	mu          sync.Mutex
	Running     bool      `json:"running"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	AccountID   string    `json:"account_id"`
	StoreID     string    `json:"store_id"`
	DateFrom    string    `json:"date_from"`
	DateTo      string    `json:"date_to"`
	Progress    string    `json:"progress"`
	RowsWritten int       `json:"rows_written"`
	Error       string    `json:"error,omitempty"`
}

// rebuildRequest is the JSON body for POST /api/rebuild-listing-daily.
type rebuildRequest struct {
	DateFrom  string `json:"date_from"`
	DateTo    string `json:"date_to"`
	AccountID string `json:"account_id,omitempty"`
	StoreID   string `json:"store_id,omitempty"`
}

// apiRebuildListingDaily starts an async listing-daily rebuild for the given
// date range. Optional account_id and store_id filter which accounts/stores
// to rebuild; empty means all.
func (s *Server) apiRebuildListingDaily(w http.ResponseWriter, r *http.Request) {
	s.rebuildStatus.mu.Lock()
	if s.rebuildStatus.Running {
		s.rebuildStatus.mu.Unlock()
		errJSON(w, http.StatusConflict, "listing-daily rebuild is already in progress")
		return
	}
	s.rebuildStatus.mu.Unlock()

	var req rebuildRequest
	if err := decodeJSON(r, &req); err != nil {
		errJSON(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}
	req.DateFrom = strings.TrimSpace(req.DateFrom)
	req.DateTo = strings.TrimSpace(req.DateTo)
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.StoreID = strings.TrimSpace(req.StoreID)

	if req.DateFrom == "" || req.DateTo == "" {
		errJSON(w, http.StatusBadRequest, "date_from and date_to are required")
		return
	}

	from, err := time.Parse("2006-01-02", req.DateFrom)
	if err != nil {
		errJSON(w, http.StatusBadRequest, fmt.Sprintf("date_from must be YYYY-MM-DD: %v", err))
		return
	}
	to, err := time.Parse("2006-01-02", req.DateTo)
	if err != nil {
		errJSON(w, http.StatusBadRequest, fmt.Sprintf("date_to must be YYYY-MM-DD: %v", err))
		return
	}
	if from.After(to) {
		errJSON(w, http.StatusBadRequest, "date_from must not be after date_to")
		return
	}

	s.rebuildStatus.mu.Lock()
	s.rebuildStatus = &rebuildStatus{
		Running:   true,
		StartedAt: time.Now(),
		AccountID: req.AccountID,
		StoreID:   req.StoreID,
		DateFrom:  req.DateFrom,
		DateTo:    req.DateTo,
		Progress:  "starting",
	}
	s.rebuildStatus.mu.Unlock()

	go s.runRebuild(context.Background(), s.dbx, s.cfg, req.AccountID, req.StoreID, from, to)

	okJSON(w, map[string]any{
		"status":     "started",
		"date_from":  req.DateFrom,
		"date_to":    req.DateTo,
		"account_id": req.AccountID,
		"store_id":   req.StoreID,
	})
}

// apiRebuildStatus returns the current state of the async rebuild.
func (s *Server) apiRebuildStatus(w http.ResponseWriter, r *http.Request) {
	s.rebuildStatus.mu.Lock()
	st := *s.rebuildStatus
	s.rebuildStatus.mu.Unlock()
	okJSON(w, st)
}

// runRebuild is the background goroutine that performs the actual rebuild
// using the shared internal/rebuild package.
func (s *Server) runRebuild(ctx context.Context, dbx *sqlx.DB, cfg *config.Config, accountID, storeID string, from, to time.Time) {
	setProgress := func(p string) {
		s.rebuildStatus.mu.Lock()
		s.rebuildStatus.Progress = p
		s.rebuildStatus.mu.Unlock()
	}
	setError := func(errMsg string) {
		s.rebuildStatus.mu.Lock()
		s.rebuildStatus.Error = errMsg
		s.rebuildStatus.mu.Unlock()
		log.Printf("[rebuild] ERROR: %s", errMsg)
	}

	defer func() {
		s.rebuildStatus.mu.Lock()
		s.rebuildStatus.Running = false
		s.rebuildStatus.CompletedAt = time.Now()
		s.rebuildStatus.mu.Unlock()
	}()

	setProgress("running")

	rowsWritten, err := rebuild.RunListingDaily(ctx, dbx, cfg, accountID, storeID, from, to, func(accID, storeID, ch string, date time.Time, rows, total int) {
		setProgress(fmt.Sprintf("processed %s/%s/%s/%s (current batch=%d, total=%d)", accID, storeID, ch, date.Format("2006-01-02"), rows, total))
	})
	if err != nil {
		setError(err.Error())
		return
	}

	s.rebuildStatus.mu.Lock()
	s.rebuildStatus.RowsWritten = rowsWritten
	s.rebuildStatus.Progress = "completed"
	s.rebuildStatus.mu.Unlock()
	log.Printf("[rebuild] listing-daily rebuild complete: %d rows from=%s to=%s", rowsWritten, from.Format("2006-01-02"), to.Format("2006-01-02"))
}