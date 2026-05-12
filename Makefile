BINARY  := model-connectivity
CMD     := ./cmd/cg
DIST    := dist
THEMES  := default argon

.PHONY: all build build-frontend build-theme-% test lint clean dev-backend dev-frontend

all: build

## 前端构建（所有主题）
build-frontend: $(addprefix build-theme-,$(THEMES))

## 单个主题构建
build-theme-%:
	cd frontend/themes/$* && npm ci && npm run build

## 完整构建（先编前端，再编后端）
build: build-frontend
	mkdir -p $(DIST)
	go build -trimpath -ldflags="-s -w" -o $(DIST)/$(BINARY) $(CMD)

## 仅后端（跳过前端，适合后端迭代开发）
build-backend:
	mkdir -p $(DIST)
	go build -o $(DIST)/$(BINARY) $(CMD)

## 运行所有测试
test:
	go test -v -race ./...

## 静态检查
lint:
	go vet ./...

## 清理构建产物
clean:
	rm -rf $(DIST) web/themes

## 本地开发 — 后端（读取 .env）
dev-backend:
	go run $(CMD)

## 本地开发 — 默认主题前端（Vite dev server，代理到后端 :8080）
dev-frontend:
	cd frontend/themes/default && npm run dev

## 本地开发 — Argon 主题前端
dev-argon:
	cd frontend/themes/argon && npm run dev
