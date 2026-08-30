# ============================================================
# Docker Manager License — 构建 Makefile
#
# 常用目标:
#   make              完整构建(前端 web/dist + 后端二进制)
#   make web          仅构建前端
#   make backend      仅构建后端(需先 make web)
#   make run          完整构建后运行(需 .env 或环境变量)
#   make keygen       生成 Ed25519 密钥对(private/ 目录)
#   make migrate      仅执行数据库迁移
#   make test         质量检查:gofmt + go vet + go test + go test -race(与 CI 一致)
#   make docker       构建 Docker 镜像
#   make up           本地 docker compose 启动(需 .env)
#   make down         停止 compose
#   make clean        清理构建产物
#   make help         查看帮助
# ============================================================

GO        ?= go
BIN       := license-server
WEB_DIR   := web

VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo unknown)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.Commit=$(GIT_COMMIT) \
	-X main.BuildTime=$(BUILD_TIME)

.PHONY: help all web backend run keygen migrate test docker up down clean

help:
	@echo "Docker Manager License Makefile"
	@echo "  make           完整构建"
	@echo "  make web       仅构建前端(web/dist)"
	@echo "  make backend   仅构建后端(需先 make web)"
	@echo "  make run       构建后运行"
	@echo "  make keygen    生成 Ed25519 密钥对"
	@echo "  make migrate   执行数据库迁移"
	@echo "  make test      gofmt + vet + test + race"
	@echo "  make docker    构建 Docker 镜像"
	@echo "  make up/down   docker compose 启停"

all: web backend

web:
	cd $(WEB_DIR) && npm install --no-audit --no-fund && npm run build

backend:
	@test -f web/dist/index.html || (echo "web/dist 不存在,请先执行 make web 构建前端"; exit 1)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN) ./cmd/license-server

run: all
	./$(BIN)

keygen:
	$(GO) run ./cmd/license-server keygen

migrate:
	$(GO) run ./cmd/license-server migrate

test:
	@echo "== gofmt =="
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then echo "未通过 gofmt:"; echo "$$unformatted"; exit 1; fi
	@echo "== go vet =="
	$(GO) vet ./...
	@echo "== go test =="
	$(GO) test ./...
	@echo "== go test -race =="
	$(GO) test -race ./internal/...

docker:
	docker build -t dockorae-auth:$(VERSION) .

up:
	docker compose -f deploy/docker-compose.yml up -d

down:
	docker compose -f deploy/docker-compose.yml down

clean:
	rm -f $(BIN)
	rm -rf $(WEB_DIR)/dist
