#!/usr/bin/env bash
# check.sh — 领星同步机「生产机自检」脚本
# 在最终部署的那台服务器上跑,一把梭确认:出口IP / 服务存活 / 领星解析连通 / 端到端触发。
# 用法:  bash check.sh            # 只做只读探测,不触发同步
#         bash check.sh --trigger  # 额外手动触发一次 sc_stores 并回看结果
set -u

PORT="${PORT:-7799}"
BASE="http://127.0.0.1:${PORT}"
LX_HOST="openapi.lingxing.com"
DO_TRIGGER=0
[ "${1:-}" = "--trigger" ] && DO_TRIGGER=1

line() { printf '\n=== %s ===\n' "$1"; }

# 1) 出口 IP —— 这才是要报给领星白名单的 IP(公网看到的源地址)
line "1. 本机出口 IP(报给领星加白名单的就是这个)"
if command -v curl >/dev/null 2>&1; then
  EGRESS="$(curl -s --max-time 5 https://api.ipify.org || true)"
  echo "ipify 直测出口 IP: ${EGRESS:-<探测失败,检查本机能否出公网>}"
  echo "同步机自报出口 IP:"
  curl -s --max-time 6 "${BASE}/api/egress-ip" || echo "  <同步机未响应,见第2步>"
  echo
else
  echo "未找到 curl,跳过"
fi

# 2) 同步机服务是否存活
line "2. 同步机服务存活(:${PORT})"
if command -v curl >/dev/null 2>&1; then
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "${BASE}/api/status" || echo 000)"
  echo "GET /api/status -> HTTP ${code}"
  [ "$code" = "200" ] && echo "服务在跑" || echo "服务未响应(未启动?端口占用?)"
fi

# 3) 领星域名解析 + TCP 443 连通(验证'广州 IP'与直连可达)
line "3. 领星域名解析(验证是否国内 IP)"
if command -v getent >/dev/null 2>&1; then
  getent hosts "$LX_HOST" || echo "getent 无结果"
elif command -v nslookup >/dev/null 2>&1; then
  nslookup "$LX_HOST" 2>&1 | sed -n '1,12p'
else
  echo "无 getent/nslookup,跳过解析"
fi
line "4. 领星 TCP 443 连通"
if command -v curl >/dev/null 2>&1; then
  curl -s -o /dev/null -w 'HTTPS 握手到 %{remote_ip} -> HTTP %{http_code} (总耗时 %{time_total}s)\n' \
    --max-time 6 "https://${LX_HOST}/" || echo "连不上(网络/防火墙/DNS 问题)"
fi

# 5) 可选:端到端触发一次最轻量的店铺接口
if [ "$DO_TRIGGER" = "1" ] && command -v curl >/dev/null 2>&1; then
  line "5. 手动触发 sc_stores"
  curl -s -X POST "${BASE}/api/sync/sc_stores"; echo
  echo "已入队。等 5s 后回看任务/落库(如仍 403,说明白名单 IP 与第1步出口 IP 不一致)"
  sleep 5
  echo "--- 最近任务(/api/status 里 sc_stores 的 last_status)---"
  curl -s --max-time 6 "${BASE}/api/status" || true
  echo
fi

line "结论提示"
cat <<'EOF'
- 领星白名单要填的,是第 1 步「出口 IP」(公网看到的源地址),不是网卡上的内网 IP。
- 若出口 IP 是共享 NAT/代理且会漂移 -> 给本机绑独享固定公网 IP(EIP),否则白名单迟早失效。
- 第 3 步解析到国内(广州)IP 只代表'对端在国内',与你的出口 IP 无关。
- 白名单生效后 bash check.sh --trigger,sc_stores 变 success、ls_stores 有行 = 端到端跑通。
EOF
