# 多阶段构建：编译阶段用完整 Go 镜像，运行阶段只留二进制，镜像体积可控。
# go.mod 要求 1.26，镜像版本通过 GO_IMAGE 可调，便于在 CI 里锁定小版本。
ARG GO_IMAGE=golang:1.26-alpine
FROM ${GO_IMAGE} AS builder

WORKDIR /src

# 先只拷依赖清单，利用层缓存避免每次改代码都重新下载依赖。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
# CGO_ENABLED=0 生成静态二进制，运行阶段才能用 alpine/scratch 这类极小基础镜像。
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/api ./cmd/api

FROM alpine:3.20

# ca-certificates：调用微信/短信等 HTTPS 接口必需
# tzdata：日志与业务时间依赖 Asia/Shanghai
RUN apk add --no-cache ca-certificates tzdata \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

# 以非 root 运行，缩小被攻破后的影响面。
RUN adduser -D -H -u 10001 appuser

WORKDIR /app
COPY --from=builder /out/api /app/api
COPY --from=builder /src/configs/config.example.yaml /app/configs/config.yaml
COPY --from=builder /src/internal/docs/openapi.yaml /app/internal/docs/openapi.yaml

RUN mkdir -p /app/data && chown -R appuser:appuser /app
USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/api"]
