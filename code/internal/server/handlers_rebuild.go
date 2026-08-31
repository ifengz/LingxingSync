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

// rebuildSnapshot 是 rebuildStatus 对外输出的纯值快照（不含互斥锁，可安全拷贝）。
type rebuildSnapshot struct {
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

// rebuildStatus 持有异步日维回刷的运行态；mu 保护 snapshot，快照不带锁。
type rebuildStatus struct {
	mu       sync.Mutex
	snapshot rebuildSnapshot
}

// rebuildRequest 是 POST /api/rebuild-listing-daily 的请求体。
type rebuildRequest struct {
	DateFrom  string `json:"date_from"`
	DateTo    string `json:"date_to"`
	AccountID string `json:"account_id,omitempty"`
	StoreID   string `json:"store_id,omitempty"`
}

// apiRebuildListingDaily 启动一次异步日维回刷。可选 account_id/store_id 过滤
// 账号/店铺；为空表示全部。同一时间只允许一次回刷在跑。
func (s *Server) apiRebuildListingDaily(w http.ResponseWriter, r *http.Request) {
	s.rebuildStatus.mu.Lock()
	if s.rebuildStatus.snapshot.Running {
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
	s.rebuildStatus.snapshot = rebuildSnapshot{
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

// apiRebuildStatus 返回异步回刷的当前状态。
func (s *Server) apiRebuildStatus(w http.ResponseWriter, r *http.Request) {
	s.rebuildStatus.mu.Lock()
	st := s.rebuildStatus.snapshot
	s.rebuildStatus.mu.Unlock()
	okJSON(w, st)
}

// runRebuild 是执行回刷的后台协程，复用 internal/rebuild 包。
func (s *Server) runRebuild(ctx context.Context, dbx *sqlx.DB, cfg *config.Config, accountID, storeID string, from, to time.Time) {
	setProgress := func(p string) {
		s.rebuildStatus.mu.Lock()
		s.rebuildStatus.snapshot.Progress = p
		s.rebuildStatus.mu.Unlock()
	}
	setError := func(errMsg string) {
		s.rebuildStatus.mu.Lock()
		s.rebuildStatus.snapshot.Error = errMsg
		s.rebuildStatus.mu.Unlock()
		log.Printf("[rebuild] ERROR: %s", errMsg)
	}

	defer func() {
		s.rebuildStatus.mu.Lock()
		s.rebuildStatus.snapshot.Running = false
		s.rebuildStatus.snapshot.CompletedAt = time.Now()
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
	s.rebuildStatus.snapshot.RowsWritten = rowsWritten
	s.rebuildStatus.snapshot.Progress = "completed"
	s.rebuildStatus.mu.Unlock()
	log.Printf("[rebuild] listing-daily rebuild complete: %d rows from=%s to=%s", rowsWritten, from.Format("2006-01-02"), to.Format("2006-01-02"))
}
