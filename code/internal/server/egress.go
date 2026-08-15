package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"lingxing-sync/internal/httptransport"
)

// egressIPURLs 是探测出口 IP 的候选源，按顺序尝试，首个成功即返回。
// 单一源（如 api.ipify.org）在某些网络环境下会被拦截导致探测失败，
// 因此保留多个独立来源做 fallback。
var egressIPURLs = []string{
	"https://api.ipify.org",
	"https://ifconfig.me",
	"https://icanhazip.com",
}

var egressHTTPClient = httptransport.NewIPv4Client(0)

type egressIPOut struct {
	IP        *string  `json:"ip"`
	Source    *string  `json:"source"`
	Sources   []string `json:"sources"`
	CheckedAt rfc3339  `json:"checked_at"`
	Error     *string  `json:"error"`
}

func (s *Server) apiEgressIP(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	out := egressIPOut{CheckedAt: rfc3339(now), Sources: egressIPURLs}

	// ?source=<url> 指定单一探测源测试；不传则按 fallback 顺序自动尝试。
	specific := r.URL.Query().Get("source")
	if specific != "" {
		ip, err := fetchEgressIPFrom(r.Context(), specific)
		if err != nil {
			message := err.Error()
			out.Error = &message
		} else {
			out.IP = &ip
			out.Source = &specific
		}
		okJSON(w, out)
		return
	}

	ip, src, err := fetchEgressIP(r.Context())
	if err != nil {
		message := err.Error()
		out.Error = &message
	} else {
		out.IP = &ip
		out.Source = &src
	}
	okJSON(w, out)
}

func fetchEgressIP(ctx context.Context) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var lastErr error
	for _, url := range egressIPURLs {
		ip, err := fetchEgressIPFrom(ctx, url)
		if err == nil {
			return ip, url, nil
		}
		lastErr = err
	}
	return "", "", lastErr
}

func fetchEgressIPFrom(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "curl/8.0.0") // 部分探测源对 Go 默认 UA 返回 HTML 而非纯 IP
	if err != nil {
		return "", fmt.Errorf("创建出口 IP 请求 (%s): %w", url, err)
	}
	resp, err := egressHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("探测出口 IP (%s): %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("探测出口 IP (%s) HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", fmt.Errorf("读取出口 IP (%s): %w", url, err)
	}
	return parseEgressIP(string(body))
}

func parseEgressIP(body string) (string, error) {
	ip := strings.TrimSpace(body)
	if _, err := netip.ParseAddr(ip); err != nil {
		return "", fmt.Errorf("出口 IP 响应不是 IP 地址: %q", ip)
	}
	return ip, nil
}
