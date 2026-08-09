#!/usr/bin/env bash
# deploy.sh — 领星同步机「服务器自动部署」脚本
# 由 GitHub Actions 调用宝塔 WebHook 触发（push main → 拉代码 → 重编 → 重启）。
# 也可手动执行：bash /www/wwwroot/lingxing-sync/code/deploy.sh
#
# 约定：
#   - 仓库根目录：REPO_DIR（默认 /www/wwwroot/lingxing-sync）
#   - Go 二进制与代码在 $REPO_DIR/code
#   - 编译产物文件名：APP（lingxing-sync，带横杠）
#   - Supervisor 守护进程名：PROG（lingxingsync，无横杠，与宝塔「进程守护管理器」里的名称一致）
#     ⚠️ APP 与 PROG 是两回事：APP 是磁盘上的二进制文件名，PROG 是 supervisor 里注册的 program 名。
#        宝塔守护进程叫 lingxingsync（无横杠），若改名务必同步这里的 PROG。
#   - 只快进拉取 main，不做任何破坏性 git 操作
set -euo pipefail

REPO_DIR="${REPO_DIR:-/www/wwwroot/lingxing-sync}"
CODE_DIR="${REPO_DIR}/code"
BRANCH="${BRANCH:-main}"
APP="${APP:-lingxing-sync}"
GIT=(git -c "safe.directory=${REPO_DIR}")
FORCE_DEPLOY="${FORCE_DEPLOY:-0}"
# Supervisor 目标名。必须是「组名:*」而不是裸程序名：
# 宝塔的 lingxingsync.ini 里 [program:lingxingsync] 带 numprocs，实际进程叫
# lingxingsync_00 并归在 lingxingsync 组下。裸 `supervisorctl restart lingxingsync`
# 会报「no such process」——不是名字拼错，是少了组语法。实测踩过这个坑。
# 用 `supervisorctl status`（不带参数）可看到真实名字形如 lingxingsync:lingxingsync_00。
PROG="${PROG:-lingxingsync:*}"
PORT="${PORT:-7799}"
LOCK_FILE="${LOCK_FILE:-/tmp/lingxing-sync-deploy.lock}"
# 健康检查探测路径。必须是一个真实存在的 GET 路由（见第 5 步注释）。
HEALTH_PATH="${HEALTH_PATH:-/api/settings}"
# supervisord 的配置文件路径。必须显式传给 supervisorctl（见第 4 步注释）：
# 裸 supervisorctl 读默认配置，可能连不到宝塔那个 supervisord 实例，
# 表现为「no such process」——程序其实好着，只是问错了人。
# 实测本机 supervisord 启动命令：supervisord -c /etc/supervisor/supervisord.conf
SUPERVISOR_CONF="${SUPERVISOR_CONF:-/etc/supervisor/supervisord.conf}"

# Go 与 supervisorctl 可能不在 WebHook/脚本的 PATH 里，显式补上常见安装路径。
# 宝塔把 supervisor 装在面板 pyenv 里（/www/server/panel/pyenv/bin），非标准 PATH，
# 交互式终端能找到但脚本环境找不到，故在此显式加入。
export PATH="/usr/local/go/bin:/usr/local/bin:/www/server/panel/pyenv/bin:${PATH}"
# Go 编译缓存/模块目录，避免落到 www 用户没权限的地方
export GOCACHE="${GOCACHE:-${CODE_DIR}/.gocache}"
# 宝塔 WebHook 不提供 HOME；Go 无法自行推导 GOPATH/GOMODCACHE，必须显式指定。
export GOPATH="${GOPATH:-/root/go}"
export GOMODCACHE="${GOMODCACHE:-${GOPATH}/pkg/mod}"

log() { printf '\n\033[1;32m[deploy %(%F %T)T]\033[0m %s\n' -1 "$*"; }
fail() { printf '\n\033[1;31m[deploy 失败]\033[0m %s\n' "$*" >&2; exit 1; }

command -v flock >/dev/null 2>&1 || fail "找不到 flock，拒绝并发部署"
exec 9>"${LOCK_FILE}"
flock -n 9 || fail "已有另一轮部署正在执行"

command -v git >/dev/null 2>&1 || fail "找不到 git"
command -v go  >/dev/null 2>&1 || fail "找不到 go（检查 /usr/local/go/bin 是否在 PATH）"

log "1/7 进入仓库：${REPO_DIR}"
cd "${REPO_DIR}" || fail "仓库目录不存在：${REPO_DIR}"

log "2/7 拉取最新代码（快进 origin/${BRANCH}）"
"${GIT[@]}" fetch --prune origin
BEFORE="$("${GIT[@]}" rev-parse --short HEAD)"
if [ -n "$("${GIT[@]}" status --porcelain --untracked-files=all)" ]; then
  fail "服务器工作树不干净，拒绝覆盖现场改动；请先人工核对 git status"
fi
"${GIT[@]}" checkout "${BRANCH}"
"${GIT[@]}" pull --ff-only origin "${BRANCH}"
AFTER="$("${GIT[@]}" rev-parse --short HEAD)"
AFTER_FULL="$("${GIT[@]}" rev-parse HEAD)"
log "版本：${BEFORE} → ${AFTER}"

if [ "${BEFORE}" = "${AFTER}" ] && [ "${FORCE_DEPLOY}" != "1" ]; then
  log "代码无变化，跳过编译与重启"
  exit 0
fi

log "3/7 运行 Go 测试"
cd "${CODE_DIR}"
go test ./... || fail "go test ./... 失败，拒绝部署"

log "4/7 运行 go vet"
go vet ./... || fail "go vet ./... 失败，拒绝部署"

log "5/7 编译二进制"
NEXT_APP="${APP}.new"
rm -f "${NEXT_APP}"
go build -ldflags="-s -w -X lingxing-sync/internal/server.BuildCommit=${AFTER_FULL}" -o "${NEXT_APP}"
test -x "./${NEXT_APP}" || fail "编译产物不存在或不可执行"

log "6/7 校验生产配置（不连接数据库、不启动服务）"
"./${NEXT_APP}" -config config.yaml -validate-config || fail "config.yaml 校验失败，拒绝部署"
mv -f "${NEXT_APP}" "${APP}"

log "7/7 重启服务（supervisor）"
# 必须带 -c：宝塔的 supervisord 是用 -c /etc/supervisor/supervisord.conf 起的，
# 裸 supervisorctl 读默认配置，可能连到另一个（或空的）实例，报「no such process」
# 而让人误以为进程名写错了。实测踩过这个坑。
command -v supervisorctl >/dev/null 2>&1 || fail "找不到 supervisorctl，请手动重启 ${PROG}"
SVC=(supervisorctl -c "${SUPERVISOR_CONF}")
if ! "${SVC[@]}" restart "${PROG}"; then
  # program 定义在宝塔插件目录（/www/server/panel/plugin/supervisor/profile/*.ini），
  # 新增或改过之后 supervisord 内存里可能还没有它 → reread 读盘、update 加载，再重试。
  log "restart 失败，reread/update 重新加载 program 定义后重试"
  "${SVC[@]}" reread || true
  "${SVC[@]}" update || true
  "${SVC[@]}" restart "${PROG}" || "${SVC[@]}" start "${PROG}" \
    || fail "supervisorctl 起不来 ${PROG}：检查 ${SUPERVISOR_CONF} 的 [include] 是否覆盖 /www/server/panel/plugin/supervisor/profile/*.ini"
fi

status_output="$("${SVC[@]}" status "${PROG}" 2>&1)" || fail "Supervisor 查询失败：${status_output}"
printf '%s\n' "${status_output}"
printf '%s\n' "${status_output}" | grep -q 'RUNNING' || fail "Supervisor 未进入 RUNNING，拒绝通过部署"

log "健康检查（:${PORT}${HEALTH_PATH}）"
# 探测点用 /api/settings：一个真实存在的 GET 路由。
# ⚠️ 勿改回 /api/status —— 该路由已随「概览页」一并删除，打它恒 404，
#    会让健康检查在服务完全正常的情况下误判部署失败。
#    换探测点时先用 `grep -n 'GET /api' internal/server/server.go` 确认路由还在。
#
# 认 200 或 401 都算「服务活着」：
#   - 未配 server.secret 时返回 200；
#   - 配了 secret 时，不带 X-Sync-Secret 头会被中间件挡成 401——
#     但 401 恰恰证明 HTTP server 已起、鉴权中间件在工作，是有效存活信号。
# 真正的宕机表现为连接拒绝（curl 返回 000），不会是 200/401。
sleep 2
for i in 1 2 3 4 5; do
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:${PORT}${HEALTH_PATH}" || echo 000)"
  if [ "${code}" = "200" ] || [ "${code}" = "401" ]; then
    log "部署完成 ✅  服务已就绪（HTTP ${code}），当前版本 ${AFTER}"
    exit 0
  fi
  sleep 2
done
fail "服务重启后 ${HEALTH_PATH} 未就绪（当前 ${code}，期望 200/401），请查 supervisor 日志"
