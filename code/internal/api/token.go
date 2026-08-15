// Package api 的 token.go 实现 TokenHolder：领星 OpenAPI access_token 的缓存与单飞刷新。
//
// 宪法对应：
//   - doc/01-architecture.md §7（同一 app_key 的所有 Worker 共用一个 TokenHolder，singleflight）
//   - doc/08-api-reference.md §2.1（取 token 接口）
//   - doc/08-api-reference.md §8（并发取 token 触发限流 → singleflight）
//
// 不引 golang.org/x/sync，用 sync.Mutex + inFlight 标志手写单飞：多个 goroutine 同时调 Get，
// 只有第一个真去 HTTP 刷，其他阻塞等同一个 done chan；结果（含错误）共享给所有等待者。
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"lingxing-sync/internal/config"
	"lingxing-sync/internal/httptransport"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"
)

// tokenEndpoint 是领星取 token 的固定路径（doc/08 §2.1）。
const tokenEndpoint = "/api/auth-server/oauth/access-token"

// tokenRefreshLeadTime 是 token 提前视为过期的保险时间（秒）。
// 领星 token 有效期 7200s，提前 60s 视为过期，避免请求到一半 token 失效。
const tokenRefreshLeadTime = 60

// TokenHolder 缓存一个领星账号的 access_token，并用单飞保护并发刷新。
// 同一 app_key 的所有 Worker 共用同一个实例（宪法 §7）。
type TokenHolder struct {
	cfg        *config.Account
	baseURL    string
	httpClient *http.Client

	mu       sync.Mutex
	token    string
	expireAt time.Time // token 实际可用到期时间（=领星返回的 expires_in - tokenRefreshLeadTime）
	inFlight bool      // 单飞标志：true 表示有 goroutine 正在刷
	checked  bool      // 是否已完成过一次真实 token 请求（成功或失败）
	done     chan struct{}
	err      error // 本次刷新的错误，共享给所有等待者
}

// tokenResponse 是领星取 token 接口的响应 JSON。
// 实测 token 接口返回 code:"200"（字符串），用 apiCode 兼容数字/字符串。
type tokenResponse struct {
	Code apiCode `json:"code"`
	Msg  string  `json:"message"` // 部分接口返回 message，部分返回 msg
	Data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"` // 秒，通常 7200
	} `json:"data"`
}

// NewTokenHolder 创建一个 TokenHolder。
// baseURL 通常是 "https://openapi.lingxing.com"。
// httpClient 为 nil 时用默认 60s 超时客户端。
func NewTokenHolder(cfg *config.Account, baseURL string, httpClient *http.Client) *TokenHolder {
	if httpClient == nil {
		httpClient = httptransport.NewIPv4Client(60 * time.Second)
	}
	return &TokenHolder{
		cfg:        cfg,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

// Get 返回有效的 access_token。
// 缓存有效直接返回；否则 singleflight 刷新（多个 goroutine 同时调只有一个真去 HTTP）。
// 刷新失败返回 error，所有等待者共享同一个 error。
func (h *TokenHolder) Get(ctx context.Context) (string, error) {
	// fast path：缓存有效直接返回（读多写少，绝大多数请求走这里）
	h.mu.Lock()
	if h.token != "" && time.Now().Before(h.expireAt) {
		t := h.token
		h.mu.Unlock()
		return t, nil
	}
	h.mu.Unlock()

	return h.refreshAndWait(ctx)
}

// ForceRefresh 强制刷新 token（不管缓存是否有效）。
// 业务请求收到 token 过期错误（40001）时调用，刷完重试一次（宪法 §8）。
func (h *TokenHolder) ForceRefresh(ctx context.Context) error {
	_, err := h.refreshAndWait(ctx)
	return err
}

// ExpiresInSec 返回当前缓存 token 的剩余有效秒数（已扣除 60s 保险）。
// 给 /api/settings 展示用；返回 0 表示从未取过 token 或已过期。
func (h *TokenHolder) ExpiresInSec() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.expireAt.IsZero() {
		return 0
	}
	d := int64(time.Until(h.expireAt).Seconds())
	if d < 0 {
		return 0
	}
	return d
}

// IsValid 返回缓存 token 是否仍有效（未过期且非空）。
func (h *TokenHolder) IsValid() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.token != "" && time.Now().Before(h.expireAt)
}

// IsKnown reports whether this holder has completed a real token request.
// It distinguishes an unverified account from a verified-but-invalid one.
func (h *TokenHolder) IsKnown() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.checked
}

// refreshAndWait 是单飞核心：第一个调用者真正刷新，后续调用者阻塞等待同一个 done chan。
// 所有等待者拿到的是同一份 token 和同一个 err（含 nil）。
func (h *TokenHolder) refreshAndWait(ctx context.Context) (string, error) {
	h.mu.Lock()
	// 有人在刷 → 挂到那个 done chan 上等结果（单飞合并）
	if h.inFlight {
		done := h.done
		h.mu.Unlock()
		select {
		case <-done:
			// 刷新结束，读取共享结果
		case <-ctx.Done():
			return "", ctx.Err()
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.err != nil {
			return "", h.err
		}
		if h.token == "" {
			return "", fmt.Errorf("lingxing token: refresh returned empty token")
		}
		return h.token, nil
	}

	// 没人在刷，我来刷：立 inFlight 标志，建 done chan。
	h.inFlight = true
	h.done = make(chan struct{})
	h.err = nil
	h.mu.Unlock()

	// 真正去 HTTP 刷（不持锁，避免阻塞其他 Get 的 fast path 与等待者）
	tok, expiresIn, err := h.fetchToken(ctx)

	h.mu.Lock()
	h.checked = true
	if err != nil {
		// 刷新失败：不覆盖旧 token（让旧 token 继续被尝试，虽然多半已过期），
		// 只把 err 存下共享给等待者。expireAt 保持原值。
		h.err = err
	} else {
		h.token = tok
		h.err = nil
		// 到期时间 = 现在 + (expires_in - 保险提前量)。expires_in 不足保险量时至少给 1s。
		real := expiresIn - tokenRefreshLeadTime
		if real < 1 {
			real = 1
		}
		h.expireAt = time.Now().Add(time.Duration(real) * time.Second)
	}
	h.inFlight = false
	close(h.done)
	h.mu.Unlock()

	if err != nil {
		return "", err
	}
	return tok, nil
}

// fetchToken 真正打 HTTP 取 token。
// 宪法 doc/08 §2.1 + doc/LINGXING_API_INTEGRATION.md §8.0：
// POST {baseURL}/api/auth-server/oauth/access-token，Content-Type: multipart/form-data，
// 字段 appId + appSecret（注意大小写）。
// 返回 access_token 与 expires_in（秒）。
func (h *TokenHolder) fetchToken(ctx context.Context) (token string, expiresIn int64, err error) {
	// 构造 multipart body（领星要求 multipart/form-data，不是 json 也不是 urlencoded）
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	if err := w.WriteField("appId", h.cfg.AppKey); err != nil {
		return "", 0, fmt.Errorf("lingxing token: write appId field: %w", err)
	}
	if err := w.WriteField("appSecret", h.cfg.AppSecret); err != nil {
		return "", 0, fmt.Errorf("lingxing token: write appSecret field: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", 0, fmt.Errorf("lingxing token: close multipart writer: %w", err)
	}

	url := h.baseURL + tokenEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return "", 0, fmt.Errorf("lingxing token: new request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("lingxing token: http do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("lingxing token: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("lingxing token: http status %d, body=%s", resp.StatusCode, truncateForLog(raw))
	}

	var tr tokenResponse
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", 0, fmt.Errorf("lingxing token: unmarshal: %w, body=%s", err, truncateForLog(raw))
	}
	if !tr.Code.isSuccess() {
		return "", 0, fmt.Errorf("lingxing token: api code=%s msg=%q", tr.Code, tr.Msg)
	}
	if tr.Data.AccessToken == "" {
		return "", 0, fmt.Errorf("lingxing token: empty access_token in response, body=%s", truncateForLog(raw))
	}

	expiresIn = tr.Data.ExpiresIn
	if expiresIn <= 0 {
		// 兜底：领星默认 7200s，若接口没返回 expires_in，按文档默认值。
		expiresIn = 7200
	}
	return tr.Data.AccessToken, expiresIn, nil
}
