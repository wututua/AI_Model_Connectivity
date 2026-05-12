# ── Stage 1: 前端构建（所有主题） ──────────────────────────────────────
FROM node:22-slim AS frontend-builder
WORKDIR /app

# default 主题
COPY frontend/themes/default/package*.json ./frontend/themes/default/
RUN cd frontend/themes/default && npm ci
COPY frontend/themes/default ./frontend/themes/default
RUN cd frontend/themes/default && npm run build

# argon 主题
COPY frontend/themes/argon/package*.json ./frontend/themes/argon/
RUN cd frontend/themes/argon && npm ci
COPY frontend/themes/argon ./frontend/themes/argon
RUN cd frontend/themes/argon && npm run build

# ── Stage 2: 后端构建 ──────────────────────────────────────────────────
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web ./web
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /model-connectivity \
    ./cmd/cg

# ── Stage 3: 最小运行镜像 ──────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=backend-builder /model-connectivity /model-connectivity
COPY --from=backend-builder /app/web ./web

EXPOSE 8080
VOLUME ["/app/data"]

ENTRYPOINT ["/model-connectivity"]
