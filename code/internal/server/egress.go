package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const egressIPURL = "https://api.ipify.org"

type egressIPOut struct {
	IP        *string `json:"ip"`
	CheckedAt rfc3339 `json:"checked_at"`
	Error     *string `json:"error"`
}

func (s *Server) apiEgressIP(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	ip, err := fetchEgressIP(r.Context())
	out := egressIPOut{CheckedAt: rfc3339(now)}
	if err != nil {
		message := err.Error()
		out.Error = &message
	} else {
		out.IP = &ip
	}
	okJSON(w, out)
}

func fetchEgressIP(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, egressIPURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建出口 IP 请求: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("探测出口 IP: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("探测出口 IP HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", fmt.Errorf("读取出口 IP: %w", err)
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
