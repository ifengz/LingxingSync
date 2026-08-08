// Package api 的 client.go 实现 OpenAPI 业务请求客户端：签名 + 公共参数 + 业务 body + 分页 + token 过期重试。
//
// 宪法对应：
//   - doc/08-api-reference.md §2.2（公共参数 app_key/access_token/timestamp/sign 放 query，业务参数放 JSON body）
//   - doc/08-api-reference.md §4（分页：offset+length，单页不超 200，has_more/total 判终止）
//   - doc/08-api-reference.md §5（限流/token 过期重试：code=40001 → ForceRefresh 后重试一次）
//   - doc/08-api-reference.md §8（fail-loud：非 0 code 带 code/msg 返回 error，不静默兜底）
//
// 纯标准库（net/http + encoding/json + strconv 等），不引第三方 HTTP 库。
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"lingxing-sync/internal/config"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultPageSize 是领星单页拉取的默认长度。宪法 §4：单页不超过 200。
const DefaultPageSize = 200

// FetchResult 是一次分页拉取的结果。
type FetchResult struct {
	List []map[string]any // data.list 展开成 []map（兼容 data 本身是数组的情况）
	// Total 优先取 data.total；data 是裸数组时取响应顶层的 total（见 §顶层 total）。
	Total          int
	HasMore        bool            // data.has_more 的值；缺失则为 false（parse 层不推断）
	HasMorePresent bool            // data 里是否"出现"了 has_more 字段（用于 worker 选择终止策略）
	Raw            json.RawMessage // 原始响应体（落 sync_task_logs 证据，可选）
}

// Client 是领星 OpenAPI 业务请求客户端。一个账号一个实例。
type Client struct {
	baseURL string
	account *config.Account
	holder  *TokenHolder
	http    *http.Client
}

// NewClient 创建一个业务客户端。
// baseURL 通常是 "https://openapi.lingxing.com"；超时固定 60s。
func NewClient(cfg *config.Account, baseURL string) *Client {
	hc := &http.Client{Timeout: 60 * time.Second}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		account: cfg,
		holder:  NewTokenHolder(cfg, baseURL, hc),
		http:    hc,
	}
}

// TokenHolder 暴露内部的 token holder，供外部查询 ExpiresInSec / IsValid（给 /api/settings）。
func (c *Client) TokenHolder() *TokenHolder {
	return c.holder
}

// apiResponse 是领星业务接口的统一响应壳。
//
// 领星 code 字段实测可能是数字也可能是字符串（如 token 接口返回 "200"），
// 用 apiCode 自定义类型兼容两种；成功判定走 isSuccess()。
type apiResponse struct {
	Code    apiCode         `json:"code"`
	Message string          `json:"message"`
	Msg     string          `json:"msg"` // 兼容部分接口用 msg
	Data    json.RawMessage `json:"data"`
	// Total 是「顶层 total」。部分接口（如 /erp/sc/data/mws/orders）把 data 直接返回成
	// 裸数组，总数放在响应顶层而不是 data 里：
	//   {"code":0,"message":"操作成功","data":[...],"total":905,"request_id":"..."}
	// 历史 bug：这里不收 total，parseFetchResult 又只拿得到 data，于是 Total 恒为 0，
	// worker 的翻页判定（has_more 缺失 → 看 offset+len>=total）直接在第 1 页终止，
	// 905 条订单静默落库成 200 条 —— 任务还显示 success，是最难发现的一类错。
	Total int `json:"total"`
}

// apiCode 兼容 JSON 里的数字与字符串 code（领星不同接口不统一）。
// 业务接口 code 多为数字 0，token 接口为字符串 "200"，统一存成字符串。
type apiCode string

// UnmarshalJSON 兼容 code 为数字或字符串两种 JSON 形态。
func (c *apiCode) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if len(s) == 0 {
		return nil
	}
	// 去掉引号（字符串形态）
	if s[0] == '"' && s[len(s)-1] == '"' {
		*c = apiCode(s[1 : len(s)-1])
		return nil
	}
	// 数字形态：直接存字面（如 0 → "0"，200 → "200"）
	*c = apiCode(s)
	return nil
}

// asInt 返回 code 的整数值（无法解析时返回 -1，仅用于错误码识别）。
func (c apiCode) asInt() int {
	n, err := strconv.Atoi(string(c))
	if err != nil {
		return -1
	}
	return n
}

// isSuccess 判定业务成功。领星成功码：业务接口多为 "0"，token 接口为 "200"。
func (c apiCode) isSuccess() bool {
	s := string(c)
	return s == "0" || s == "200" || s == "success"
}

// isEmptyRawData 判断响应壳里的 data 是否为空（null 或缺省）。
// 仅 null/空视为空；data:{}、data:[] 不算空（属合法的"无数据"结构）。
func isEmptyRawData(data json.RawMessage) bool {
	s := strings.TrimSpace(string(data))
	return s == "" || s == "null"
}

// isSuccessMessage 判断 msg/message 文案是否属于「成功」语义。
// 领星成功响应有时把 msg 填成 "success"/"ok" 之类；这类不应被软失败闸误判。
func isSuccessMessage(msg string) bool {
	s := strings.ToLower(strings.TrimSpace(msg))
	return s == "success" || s == "ok" || s == "成功"
}

// Fetch 拉取一页。
//   - method 来自 endpoint（GET/POST），path 是 endpoint.Path
//   - params 是业务参数（含 offset/length 分页、extra_params、sid 等）
//   - 宪法 §2.2：公共参数 app_key/access_token/timestamp/sign 放 query；POST 业务参数放 JSON body，GET 业务参数也拼到 query
//
// 返回 (result, httpStatus, apiCode, error)：
//   - httpStatus 给 Worker 写 sync_task_logs
//   - apiCode 同上
//   - error 非 nil 时，result 为 nil；error 内含 code/msg，fail-loud
func (c *Client) Fetch(ctx context.Context, method, path string, params map[string]any) (*FetchResult, int, int, error) {
	return c.FetchWithShape(ctx, method, path, params, "list")
}

// FetchWithShape 拉取一页，并按 endpoint 声明的响应形态解析 data。
// responseShape 为空时按 list 处理；object 仅用于 data 是单个业务对象的接口。
func (c *Client) FetchWithShape(ctx context.Context, method, path string, params map[string]any, responseShape string) (*FetchResult, int, int, error) {
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		m = http.MethodPost // 领星业务接口默认 POST
	}

	result, httpStatus, apiCode, err := c.fetchOnce(ctx, m, path, params, responseShape)
	if err != nil {
		// token 过期类错误：强制刷新后重试一次（宪法 §8）。
		// fetchOnce 返回的 err 在 token 过期段会带 sentinel errTokenExpired。
		if isTokenExpiredErr(err) {
			if ferr := c.holder.ForceRefresh(ctx); ferr != nil {
				return nil, httpStatus, apiCode, fmt.Errorf("lingxing fetch: token refresh failed after expired (refresh err: %v): %w", ferr, err)
			}
			return c.fetchOnce(ctx, m, path, params, responseShape)
		}
		return nil, httpStatus, apiCode, err
	}
	return result, httpStatus, apiCode, nil
}

// fetchOnce 执行一次完整的请求（签名 → 发送 → 解析），不做 token 重试。
func (c *Client) fetchOnce(ctx context.Context, method, path string, params map[string]any, responseShape string) (*FetchResult, int, int, error) {
	// 1. 取 access_token
	token, err := c.holder.Get(ctx)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("lingxing fetch: get token: %w", err)
	}

	// 2. 时间戳（Unix 秒）
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	// 3. 业务参数 string 化（用于签名）
	strParams := make(map[string]string, len(params)+4)
	for k, v := range params {
		strParams[k] = anyToString(v)
	}

	// 4. 组装 query 公共参数 + 业务签名入参
	//    签名入参 = 业务参数 + app_key + access_token + timestamp（宪法 §2.2）
	q := url.Values{}
	q.Set("app_key", c.account.AppKey)
	q.Set("access_token", token)
	q.Set("timestamp", ts)

	signInput := make(map[string]string, len(strParams)+3)
	for k, v := range strParams {
		signInput[k] = v
	}
	signInput["app_key"] = c.account.AppKey
	signInput["access_token"] = token
	signInput["timestamp"] = ts

	sign := Sign(signInput, c.account.AppKey, c.account.AppSecret)
	q.Set("sign", sign) // sign 进 query，但 sign 本身不参与签名（上面已算完）

	// 5. 构造请求
	var bodyReader io.Reader
	if method == http.MethodGet {
		// GET：业务参数也拼到 query（领星部分 GET 接口走 /erp/sc/...，业务参数在 query）
		for k, v := range strParams {
			q.Set(k, v)
		}
	} else {
		// POST：业务参数放 JSON body
		bodyBytes, err := json.Marshal(params)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("lingxing fetch: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	reqURL := c.baseURL + path
	if encoded := q.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("lingxing fetch: new request: %w", err)
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	// 6. 发送
	resp, err := c.http.Do(req)
	if err != nil {
		fetchErr := NewFetchError(0, 0, fmt.Sprintf("http do %s %s: %v", method, path, err), 0, transportMayHaveReachedUpstream(err))
		fetchErr.Cause = err
		return nil, 0, 0, fetchErr
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		fetchErr := NewFetchError(resp.StatusCode, 0, fmt.Sprintf("read body: %v", err), parseRetryAfterHeader(resp), true)
		fetchErr.Cause = err
		return nil, resp.StatusCode, 0, fetchErr
	}

	// 7. HTTP 非 200 → error（fail-loud）
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, 0, NewFetchError(resp.StatusCode, 0,
			fmt.Sprintf("http status %d for %s %s, body=%s", resp.StatusCode, method, path, truncateForLog(raw)),
			parseRetryAfterHeader(resp), true)
	}

	// 8. 解析响应壳
	var ar apiResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return nil, resp.StatusCode, 0, fmt.Errorf("lingxing fetch: unmarshal response: %w, body=%s", err, truncateForLog(raw))
	}

	// 9. code 判定（领星 code 可能是 "0" 或 "200"，统一走 isSuccess）
	if !ar.Code.isSuccess() {
		msg := ar.Message
		if msg == "" {
			msg = ar.Msg
		}
		// token 过期类错误（40001 或 message 含 token/expire）：返回 sentinel，让上层重试
		if isTokenExpiredCode(ar.Code.asInt(), msg) {
			fetchErr := NewFetchError(resp.StatusCode, ar.Code.asInt(), msg, parseRetryAfterHeader(resp), true)
			fetchErr.Cause = errTokenExpired
			return nil, resp.StatusCode, ar.Code.asInt(), fetchErr
		}
		// 其他失败：fail-loud 带 code/msg
		return nil, resp.StatusCode, ar.Code.asInt(), NewFetchError(resp.StatusCode, ar.Code.asInt(),
			fmt.Sprintf("api error code=%s msg=%q path=%s", ar.Code, msg, path), parseRetryAfterHeader(resp), true)
	}

	// 9b. 软失败闸（fail-loud，宪法 §5）：
	// 领星部分参数校验错误会返回 code=0（看似成功）但 data=null 且 msg 带错误文案，
	// 例如缺 summary_field 时 msg="[summary_field 不能为空,...]"。仅信 code 会把失败
	// 静默记成成功 0 条。判定：data 为 null/空 且 msg/message 非空且非成功词 → 业务错误。
	// 只收紧 data:null 这一种；data:{}/data:[]/空 msg 的正常空结果不受影响。
	if isEmptyRawData(ar.Data) {
		softMsg := ar.Message
		if softMsg == "" {
			softMsg = ar.Msg
		}
		if softMsg != "" && !isSuccessMessage(softMsg) {
			return nil, resp.StatusCode, ar.Code.asInt(), NewFetchError(resp.StatusCode, ar.Code.asInt(),
				fmt.Sprintf("api soft-error code=%s msg=%q path=%s", ar.Code, softMsg, path), parseRetryAfterHeader(resp), true)
		}
	}

	// 10. 解析 data → FetchResult
	result, err := parseFetchResultWithShape(ar.Data, responseShape)
	if err != nil {
		return nil, resp.StatusCode, ar.Code.asInt(), fmt.Errorf("lingxing fetch: parse data: %w, body=%s", err, truncateForLog(raw))
	}
	// 10b. 顶层 total 兜底（见 applyTopLevelTotal）。
	applyTopLevelTotal(result, ar.Total)
	if err := normalizeStoreRows(path, result.List); err != nil {
		return nil, resp.StatusCode, ar.Code.asInt(), fmt.Errorf("lingxing fetch: map store fields: %w, body=%s", err, truncateForLog(raw))
	}
	result.Raw = raw
	return result, resp.StatusCode, ar.Code.asInt(), nil
}

// applyTopLevelTotal 在 data 内没给 total 时，用响应顶层的 total 兜底。
//
// 为什么需要：领星有两种分页信号摆放方式 ——
//
//	A) data 是对象：{"data":{"list":[...],"total":905,"has_more":false}}
//	B) data 是裸数组，总数在顶层：{"data":[...],"total":905}   ← /erp/sc/data/mws/orders
//
// 形态 B 下 parseFetchResult 只拿得到 data，Total 恒为 0，worker 的翻页判定
// （has_more 缺失 → 看 offset+len>=total）在第 1 页就终止，905 条静默落成 200 条，
// 任务却是 success —— 最难发现的一类错。
//
// 优先级：data.total 高于顶层 total（data 内的更贴近该次分页语义）；
// 顶层 total 只在 data 未提供时生效。通用机制，不给单接口写死（宪法：加接口零代码）。
func applyTopLevelTotal(result *FetchResult, topLevelTotal int) {
	if result == nil {
		return
	}
	if result.Total == 0 && topLevelTotal > 0 {
		result.Total = topLevelTotal
	}
}

// parseFetchResult 把 data（json.RawMessage）解析成 FetchResult。
//
// fail-loud 原则（CLAUDE.md §3 / 宪法 §5）：领星返回格式异常时必须报错，不猜测字段名、
// 不推断分页、不静默兜底成 0 行。默认 list 模式的合法形态只有两种：
//   - 对象 {"list":[...], "total":N, "has_more":bool}（标准分页响应）
//   - 数组 [{...}, ...]（少数接口直接返回数组）
//
// data 为空对象/数组时返回空 List（属合法的"无数据"）；data 是对象但缺 list 字段、
// 或既非对象非数组时，返回 error。单对象接口必须显式使用 response_shape=object。
func parseFetchResult(data json.RawMessage) (*FetchResult, error) {
	return parseFetchResultWithShape(data, "list")
}

// parseFetchResultWithShape 解析分页列表或显式声明的单对象响应。
// 默认 list 模式保持 fail-loud，避免把上游字段变更误当成一行业务数据。
func parseFetchResultWithShape(data json.RawMessage, responseShape string) (*FetchResult, error) {
	r := &FetchResult{List: []map[string]any{}}
	if len(data) == 0 || string(data) == "null" {
		return r, nil
	}
	responseShape = strings.ToLower(strings.TrimSpace(responseShape))
	if responseShape == "" {
		responseShape = "list"
	}
	if responseShape == "object" {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, fmt.Errorf("data is not a single object: %w", err)
		}
		if len(obj) == 0 {
			return r, nil
		}
		if _, ok := obj["list"]; ok {
			return nil, fmt.Errorf("object response contains pagination field 'list'; use response_shape=list")
		}
		for _, key := range []string{"total", "has_more", "hasMore"} {
			if _, ok := obj[key]; ok {
				return nil, fmt.Errorf("object response contains pagination field %q; use response_shape=list", key)
			}
		}
		var row map[string]any
		if err := json.Unmarshal(data, &row); err != nil {
			return nil, fmt.Errorf("unmarshal single object: %w", err)
		}
		r.List = []map[string]any{row}
		r.Total = 1
		return r, nil
	}
	if responseShape != "list" {
		return nil, fmt.Errorf("unsupported response shape %q", responseShape)
	}

	// 形态一：data 是对象 {list, total, has_more}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err == nil {
		// list 字段（领星标准字段名，不猜别名）
		if listRaw, ok := obj["list"]; ok && len(listRaw) > 0 {
			var arr []map[string]any
			if err := json.Unmarshal(listRaw, &arr); err != nil {
				return nil, fmt.Errorf("unmarshal data.list: %w", err)
			}
			r.List = arr
		} else if len(obj) == 0 {
			// 空对象 {}：视为合法的"无数据"，放行返回空 List。
			return r, nil
		} else {
			// 对象有字段但缺 list：fail-loud。避免领星把分页字段改名后静默吞成 0 行假成功。
			return nil, fmt.Errorf("data is object but missing 'list' field; keys present: %v", keysOf(obj))
		}

		// total（领星标准字段）
		r.Total = readInt(obj, "total")
		// has_more：领星标准字段；无该字段视为无更多（parse 层不推断）。
		// 另记录字段是否"出现"，供 worker 决定用 has_more 还是 offset+len>=total 终止（宪法 §4）。
		if _, ok := obj["has_more"]; ok {
			r.HasMorePresent = true
		} else if _, ok := obj["hasMore"]; ok {
			r.HasMorePresent = true
		}
		if readBool(obj, "has_more", "hasMore") {
			r.HasMore = true
		}
		return r, nil
	}

	// 形态二：data 本身是数组（少数接口直接返回数组，无分页壳 → 无 total/has_more 信号）
	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err == nil {
		r.List = arr
		r.HasMore = false
		return r, nil
	}

	return nil, fmt.Errorf("data is neither object nor array: %s", truncateForLog(data))
}

// keysOf 返回 map 的 key 列表，用于 fail-loud 错误信息中展示实际存在的字段。
func keysOf(obj map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return keys
}

// errTokenExpired 是 token 过期的 sentinel error，Fetch 据此触发「刷新一次后重试」。
var errTokenExpired = fmt.Errorf("lingxing token expired")

func isTokenExpiredErr(err error) bool {
	return err != nil && (errors.Is(err, errTokenExpired) || strings.Contains(err.Error(), errTokenExpired.Error()))
}

// isTokenExpiredCode 判断 api code/msg 是否属于 token 过期段。
// 领星 token 过期常见 code=40001；message 里含 "token" / "expire"（大小写不敏感）也按过期处理。
// 宪法 §8：宁可误判一次触发刷新（最多多一次请求），也不要漏判导致任务失败。
func isTokenExpiredCode(code int, msg string) bool {
	if code == 40001 {
		return true
	}
	if code == 2001003 || code == 2001005 {
		return true
	}
	low := strings.ToLower(msg)
	return strings.Contains(low, "token") || strings.Contains(low, "expire")
}

func parseRetryAfterHeader(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	delay, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	if !ok {
		return 0
	}
	return delay
}

func transportMayHaveReachedUpstream(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// anyToString 把任意类型转成参与签名的字符串。
// 时间/数字/布尔等用 fmt.Sprintf；[]byte 转 string；其他 fallback fmt.Sprintf。
func anyToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case float64:
		// 整数浮点去掉小数点（领星 offset/length 多为 float64 经 yaml 解析）
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	case []byte:
		return string(x)
	case []any, map[string]any:
		// 数组/对象参数参与签名时按紧凑 JSON 编码（领星官方 SDK 对非标量值 json_encode），
		// 与 POST body 的 json.Marshal 形态一致，保证「签名串」和「实际 body」对得上；
		// 否则 %v 会得到 "[1]"，签名与领星侧算出的不一致 → 签名错。
		if b, err := json.Marshal(x); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

// readInt 从 json.RawMessage map 里按优先顺序读第一个存在的 int 字段。
func readInt(obj map[string]json.RawMessage, keys ...string) int {
	for _, k := range keys {
		if raw, ok := obj[k]; ok && len(raw) > 0 {
			var n json.Number
			// json.Number 需要 Decoder.UseNumber();这里直接 unmarshal 到 any 更稳
			var v any
			if err := json.Unmarshal(raw, &v); err == nil {
				switch t := v.(type) {
				case float64:
					return int(t)
				case string:
					if n, err := strconv.Atoi(t); err == nil {
						return n
					}
				}
			}
			_ = n
		}
	}
	return 0
}

// readBool 从 json.RawMessage map 里按优先顺序读第一个存在的 bool 字段。
func readBool(obj map[string]json.RawMessage, keys ...string) bool {
	for _, k := range keys {
		if raw, ok := obj[k]; ok && len(raw) > 0 {
			var b bool
			if err := json.Unmarshal(raw, &b); err == nil {
				return b
			}
		}
	}
	return false
}

// truncateForLog 把响应体截断到 512 字节，避免日志爆炸（落 sync_task_logs 证据时用）。
func truncateForLog(b []byte) string {
	const max = 512
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
