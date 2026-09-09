# ── Stage 1: 前端构建 ──────────────────────────────────────────────────
FROM node:20-slim AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ── Stage 2: 后端构建 ──────────────────────────────────────────────────
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web ./web
RUN mkdir -p /runtime-data
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
COPY --chown=65532:65532 --from=backend-builder /runtime-data ./data

ENV APP_HOST=0.0.0.0 APP_PORT=8080
EXPOSE 8080
VOLUME ["/app/data"]

ENTRYPOINT ["/model-connectivity"]
