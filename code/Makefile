# 领星同步机 Makefile（宪法 doc/00 §快速启动、doc/06 §编译）
# 本地：make tidy && make build && make run
# 服务器：make build && supervisorctl restart lingxing-sync

BIN := lingxing-sync
SRC := .
LDFLAGS := -ldflags="-s -w"

.PHONY: tidy build run migrate fmt vet clean

# 拉依赖
tidy:
	go mod tidy

# 编译单二进制（约 15MB，零运行时依赖）
build:
	go build $(LDFLAGS) -o $(BIN)

# 前台运行（默认读 code/config.yaml）
run: build
	./$(BIN) -config config.yaml

# 本地前台运行与生产使用同一真实 API 路径
dev: build
	./$(BIN) -config config.yaml

# 执行迁移（幂等：CREATE TABLE IF NOT EXISTS）
migrate:
	@echo "迁移由程序启动时自动执行（migrate.go），无需单独命令；如需手动："
	@echo "  docker exec -i lingsync-mysql mysql -uroot -prootpass lingsync < migrations/001_system.sql"

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -f $(BIN)
