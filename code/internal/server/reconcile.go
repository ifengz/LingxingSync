// internal/server/reconcile.go — 对账（reconcile）handler 与 CSV 解析 helper。
//
// 宪法对应：doc/04-api.md 的 `POST /api/reconcile`。
// 本期实现：解析 multipart 上传，读取 CSV 表头与行数，返回 ok 占位结构；
// 真正的 diff（与 DB 数据逐行比对）放到二期。
//
// 返回结构（与 handler 保持一致，便于前端稳定联调）：
//
//	{
//	  "ok": true,
//	  "data": {
//	    "matched": 0,
//	    "missing_in_db": 0,
//	    "extra_in_report": 0,
//	    "report_rows": <int>,   // 本期新增：上传 CSV 的数据行数
//	    "columns": [...],       // 本期新增：CSV 表头
//	    "diffs": []
//	  }
//	}

package server

import (
	"encoding/csv"
	"io"
	"net/http"
	"strconv"
)

// reconcileOut 是 /api/reconcile 的 data 字段结构。
type reconcileOut struct {
	Matched       int      `json:"matched"`
	MissingInDB   int      `json:"missing_in_db"`
	ExtraInReport int      `json:"extra_in_report"`
	ReportRows    int      `json:"report_rows"`
	Columns       []string `json:"columns"`
	Diffs         []any    `json:"diffs"`
}

// apiReconcile 接收 multipart 上传（字段名 file），解析 CSV，返回占位对账结果。
//
// 一期不做与 DB 的逐行 diff，只校验文件可读、回显行数与列名。
// 这样前端联调可以稳定，后续 diff 逻辑替换不影响契约。
func (s *Server) apiReconcile(w http.ResponseWriter, r *http.Request) {
	// 限制上传体积 8MB，防止单机内存被打爆
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		errJSON(w, http.StatusBadRequest, "解析 multipart 失败: "+err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		errJSON(w, http.StatusBadRequest, "缺少上传字段 file")
		return
	}
	defer file.Close()

	cols, rows, err := readCSVHeadAndCount(file)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "解析 CSV 失败: "+err.Error())
		return
	}

	okJSON(w, reconcileOut{
		Matched:       0,
		MissingInDB:   0,
		ExtraInReport: 0,
		ReportRows:    rows,
		Columns:       cols,
		Diffs:         []any{},
	})
}

// readCSVHeadAndCount 读 CSV，返回表头与数据行数。
// 若文件为空（无任何行），返回空表头与 0 行而不报错。
func readCSVHeadAndCount(rd io.Reader) (cols []string, rows int, err error) {
	r := csv.NewReader(rd)
	r.FieldsPerRecord = -1 // 允许列数不齐（容错），后续 diff 阶段再严格校验

	first, err := r.Read()
	if err == io.EOF {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	cols = first

	for {
		_, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return cols, rows, err
		}
		rows++
	}
	return cols, rows, nil
}

// （保留：未来按表名走 db.GetTableColumns 与逐行 diff 时使用）
var _ = strconv.Atoi
