#!/usr/bin/env bash
# deploy.sh — 领星同步机「服务器自动部署」脚本
# 由宝塔 WebHook 触发（GitHub push → 服务器拉代码 → 重编 → 重启）。
# 也可手动执行：bash /www/wwwroot/lingxing-sync/code/deploy.sh
#
# 约定：
#   - 仓库根目录：REPO_DIR（默认 /www/wwwroot/lingxing-sync）
#   - Go 二进制与代码在 $REPO_DIR/code
#   - Supervisor 进程名：lingxing-sync
#   - 只快进拉取 main，不做任何破坏性 git 操作
set -euo pipefail

REPO_DIR="${REPO_DIR:-/www/wwwroot/lingxing-sync}"
CODE_DIR="${REPO_DIR}/code"
BRANCH="${BRANCH:-main}"
APP="${APP:-lingxing-sync}"
PORT="${PORT:-7799}"

# Go 与 supervisorctl 可能不在 WebHook/脚本的 PATH 里，显式补上常见安装路径。
# 宝塔把 supervisor 装在面板 pyenv 里（/www/server/panel/pyenv/bin），非标准 PATH，
# 交互式终端能找到但脚本环境找不到，故在此显式加入。
export PATH="/usr/local/go/bin:/usr/local/bin:/www/server/panel/pyenv/bin:${PATH}"
# Go 编译缓存/模块目录，避免落到 www 用户没权限的地方
export GOCACHE="${GOCACHE:-${CODE_DIR}/.gocache}"

log() { printf '\n\033[1;32m[deploy %(%F %T)T]\033[0m %s\n' -1 "$*"; }
fail() { printf '\n\033[1;31m[deploy 失败]\033[0m %s\n' "$*" >&2; exit 1; }

command -v git >/dev/null 2>&1 || fail "找不到 git"
command -v go  >/dev/null 2>&1 || fail "找不到 go（检查 /usr/local/go/bin 是否在 PATH）"

log "1/5 进入仓库：${REPO_DIR}"
cd "${REPO_DIR}" || fail "仓库目录不存在：${REPO_DIR}"

log "2/5 拉取最新代码（快进 origin/${BRANCH}）"
git fetch --prune origin
BEFORE="$(git rev-parse --short HEAD)"
git checkout "${BRANCH}"
git pull --ff-only origin "${BRANCH}"
AFTER="$(git rev-parse --short HEAD)"
log "版本：${BEFORE} → ${AFTER}"

if [ "${BEFORE}" = "${AFTER}" ]; then
  log "代码无变化，跳过编译与重启"
  exit 0
fi

log "3/5 编译二进制"
cd "${CODE_DIR}"
go build -ldflags="-s -w" -o "${APP}"

log "4/5 重启服务（supervisor）"
if command -v supervisorctl >/dev/null 2>&1; then
  supervisorctl restart "${APP}" || fail "supervisorctl restart 失败"
else
  fail "找不到 supervisorctl，请手动重启 ${APP}"
fi

log "5/5 健康检查（:${PORT}/api/status）"
# 认 200 或 401 都算「服务活着」：
#   - 未配 server.secret 时 /api/status 返回 200；
#   - 配了 secret 时，不带 X-Sync-Secret 头会被中间件挡成 401——
#     但 401 恰恰证明 HTTP server 已起、鉴权中间件在工作，是有效存活信号。
# 真正的宕机表现为连接拒绝（curl 返回 000），不会是 200/401。
sleep 2
for i in 1 2 3 4 5; do
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:${PORT}/api/status" || echo 000)"
  if [ "${code}" = "200" ] || [ "${code}" = "401" ]; then
    log "部署完成 ✅  服务已就绪（HTTP ${code}），当前版本 ${AFTER}"
    exit 0
  fi
  sleep 2
done
fail "服务重启后 /api/status 未就绪（当前 ${code}，期望 200/401），请查 supervisor 日志"
