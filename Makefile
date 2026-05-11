BINARY  := model-connectivity
CMD     := ./cmd/cg
DIST    := dist

.PHONY: all build build-frontend test lint clean dev-backend dev-frontend

all: build

## 前端构建
build-frontend:
	cd frontend && npm ci && npm run build

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
	rm -rf $(DIST) frontend/dist

## 本地开发 — 后端（读取 .env）
dev-backend:
	go run $(CMD)

## 本地开发 — 前端（Vite dev server，代理到后端 :8080）
dev-frontend:
	cd frontend && npm run dev
