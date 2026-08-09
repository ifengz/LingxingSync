# LingxingSync 部署闸门

## 当前结论

线上宝塔 WebHook 已配置，但最近一次调用没有完成部署：Git 在 `/www/wwwroot/lingxing-sync` 报 `detected dubious ownership`，流程停在拉取阶段。线上 Supervisor 当前仍是 `RUNNING`，固定入口 `/api/settings` 返回 `200`，这只能证明旧版本服务存活，不能证明 push 后部署成功。

修复后的唯一链路：

```text
push main
  -> GitHub Actions（Deploy To Baota）
  -> 宝塔 WebHook（sync.usfan.net）
  -> /www/wwwroot/lingxing-sync/code/deploy.sh
  -> 拉取 main、测试、vet、编译、校验 config.yaml
  -> Supervisor 重启
  -> 127.0.0.1:7799/api/settings 健康检查
```

GitHub 原生 WebHook 不直接访问宝塔面板：面板使用自签名证书，GitHub 会拒绝投递。Actions 按同服务器现有项目的方式调用宝塔 WebHook，仓库 Secret `BAOTA_WEBHOOK_URL` 保存完整回调地址；Secret 不写入 workflow、日志或仓库。

## 宝塔 WebHook

WebHook 应只执行固定脚本：

```bash
bash /www/wwwroot/lingxing-sync/code/deploy.sh \
  >> /www/wwwroot/lingxing-sync/code/logs/deploy.log 2>&1
```

面板中确认：

- 脚本路径和上面完全一致；
- WebHook 用户能读写仓库、编译目录和 Supervisor 日志目录。

GitHub 仓库中确认：

- `.github/workflows/deploy-baota.yml` 只监听 `main` push；
- Actions Secret `BAOTA_WEBHOOK_URL` 已配置；
- Actions 调用 WebHook 后继续检查公网 `/api/settings`，只接受 `200` 或 `401`。

## 服务器一次性前置条件

```bash
cd /www/wwwroot/lingxing-sync
git remote -v
git checkout main
git status --short
chmod +x code/deploy.sh
```

服务器执行用户必须能非交互完成：

```bash
git -c safe.directory=/www/wwwroot/lingxing-sync fetch --prune origin
```

脚本已经对每个 Git 命令使用临时 `safe.directory`，不会修改全局 Git 配置。它仍会拒绝任何 tracked 或非忽略的未跟踪改动，不会替线上人员自动 stash、reset、clean 或覆盖文件。当前线上已观察到 `code/migrations/009_rename_account.sql` 有 tracked 修改；该项必须由负责人先核对并清理，脚本才会继续。

仓库 remote 也必须能以 WebHook 执行用户访问 GitHub，不能依赖人工输入密码。数据库密码、领星密钥和 `config.yaml` 只留在服务器，不进入 Git。

Supervisor 必须使用宝塔实际配置：

```text
配置文件：/etc/supervisor/supervisord.conf
目标：    lingxingsync:*
端口：    7799
健康：    /api/settings
```

如果配置是 `[program:lingxingsync]` 加 `numprocs`，实际进程名通常是 `lingxingsync:lingxingsync_00`。不要把脚本目标改回裸名 `lingxingsync`。

## 每次部署的硬闸门

`code/deploy.sh` 按顺序执行：

1. `flock` 阻止并发部署。
2. `git fetch`、检查工作树、切换 `main`、`git pull --ff-only`。
3. `go test ./...` 和 `go vet ./...`。
4. 编译 `lingxing-sync`，确认产物可执行。
5. 执行 `./lingxing-sync -config config.yaml -validate-config`，只校验配置，不连接数据库、不启动服务；`database.host/user/db` 为空会在重启前失败。
6. 使用宝塔实际 `supervisord` 配置重启 `lingxingsync:*`，并确认状态包含 `RUNNING`。
7. 检查 `http://127.0.0.1:7799/api/settings`，只接受 `200` 或 `401`。

任一步失败，脚本返回非零，WebHook 日志标红，不继续下一步。程序启动时的数据库迁移失败保持 fail-loud，不能让半完成迁移的进程继续对外服务。

## 缺表与缺列

- **仓库已有迁移文件的表/列**：程序启动时自动执行 `migrations/*.sql`，不需要手工逐条建表。
- **配置引用了仓库没有迁移文件的表**：部署不会猜测表结构；该 endpoint 会在启动时标记为不可同步，HTTP 和其他 endpoint 仍可用。需要先确认真实接口合同，再新增 migration，随后重新部署。
- **表存在但缺少配置声明的列**：当前实现只产生可见告警并继续同步，缺失字段不会落库；不会自动 `ALTER TABLE`，因为列类型、是否允许 NULL、唯一键都不能靠字段名安全推断。需要补经过验证的 migration，再重新部署。
- **迁移 SQL 本身执行失败**：部署脚本和程序都 fail-loud，停止继续发布；这不是让人临时在生产手工补一列后掩盖的信号，先看具体 migration 和 schema 差异。

因此，新增接口的正确动作是：确认真实响应字段和唯一键 → 提交 migration + 配置 → push `main` → WebHook 自动拉取、迁移、编译、重启和健康检查。

## Push 前本地检查

```bash
cd code
gofmt -l .
go test ./...
go vet ./...
go build -ldflags="-s -w" -o lingxing-sync
./lingxing-sync -config config.yaml -validate-config
cd ..
git diff --check
git status --short
```

只确认目标改动后再 push `main`。`config.yaml`、二进制、日志和备份文件不得进入 commit。

## 首次修复脏线上仓库

当前线上旧脚本会在 `git fetch` 前因 dubious ownership 失败，所以第一次必须手工执行。以下命令只针对明确文件，先备份再恢复，不使用 `reset --hard`、`clean` 或自动覆盖：

```bash
cd /www/wwwroot/lingxing-sync
BACKUP_DIR=/www/backup/lingxing-sync-20260809
mkdir -p "${BACKUP_DIR}"

# 先确认目标和当前状态，不读取 config.yaml 内容
git -c safe.directory=/www/wwwroot/lingxing-sync status --short --untracked-files=all
git -c safe.directory=/www/wwwroot/lingxing-sync fetch --prune origin
git -c safe.directory=/www/wwwroot/lingxing-sync diff -- code/migrations/009_rename_account.sql

# 备份线上临时迁移改动，再恢复该单个 tracked 文件
cp -a code/migrations/009_rename_account.sql "${BACKUP_DIR}/009_rename_account.sql"
git -c safe.directory=/www/wwwroot/lingxing-sync restore \
  --source=origin/main -- code/migrations/009_rename_account.sql

# 配置备份移出仓库，禁止直接打印或删除其内容；文件名按 status 的实际结果替换
mv code/config.yaml.bak.20260808164148 "${BACKUP_DIR}/config.yaml.bak.20260808164148"

git -c safe.directory=/www/wwwroot/lingxing-sync status --short --untracked-files=all
git -c safe.directory=/www/wwwroot/lingxing-sync pull --ff-only origin main
FORCE_DEPLOY=1 BRANCH=main bash code/deploy.sh
```

如果 `009` 的 diff 不是临时补丁，先停在这里，不要执行 `restore`；把备份内容转成正式 migration commit 后再部署。`FORCE_DEPLOY=1` 只用于这次 bootstrap，之后正常 push 由 WebHook 触发即可。

## 部署后固定入口复核

```bash
curl --fail --silent --show-error --max-time 10 \
  --output /dev/null --write-out '%{http_code} %{time_total}\n' \
  https://sync.usfan.net/api/settings
```

预期 HTTP `200` 或鉴权开启时的 `401`。公网 `502` 不能用“Supervisor 重启过”解释为成功，必须回到 WebHook 日志、Supervisor 日志和本机 7799 监听证据。

## 本次故障对应的防线

- `database.host/user/db` 为空：配置校验在重启前失败。
- 账号改名目标主键已存在：迁移只改无冲突行，冲突原地保留并记录；相关账号+接口显示不可用，不删除数据。
- Git dubious ownership：Git 命令使用仓库级临时 `safe.directory`，同时拒绝脏工作树。
- Supervisor 查询名错误：脚本固定带 `-c` 并使用 `lingxingsync:*`。
- 健康路由错误：脚本固定检查真实存在的 `/api/settings`，不使用已删除的 `/api/status`。

## 本轮线上证据

目标主机 `38.246.250.228`，时间 `2026-08-09 01:48`。宝塔 WebHook `sync.usfan.net` 已存在，最近调用一次但在 Git 拉取阶段失败；线上 `lingxingsync:lingxingsync_00` 为 `RUNNING`，本机 `/api/settings` 为 `401`，公网固定入口为 `200`。线上还存在 tracked 的 `009` 修改和配置备份未跟踪文件，未执行清理、重启或其他生产写操作。
