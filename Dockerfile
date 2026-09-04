# syntax=docker/dockerfile:1.7

ARG ALPINE_MIRROR=mirrors.aliyun.com
ARG GITHUB_REWRITE_BASE=github.com

# ===== 阶段1: 前端构建 =====
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install --legacy-peer-deps --registry=https://registry.npmmirror.com
COPY frontend/ ./
RUN npm run build

# ===== 阶段2: 后端构建 =====
FROM golang:1.25-alpine AS builder
ARG ALPINE_MIRROR
ARG GITHUB_REWRITE_BASE
RUN sed -i "s/dl-cdn.alpinelinux.org/${ALPINE_MIRROR}/g" /etc/apk/repositories \
    && apk add --no-cache git ca-certificates \
    && update-ca-certificates
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,https://goproxy.io,direct
ENV GOPRIVATE=github.com/xiuxianjs/*
COPY go.mod go.sum ./
# The token is only needed when a private GitHub module is present.  Mark the
# mount optional so ordinary public-module builds do not require a secret.
RUN --mount=type=secret,id=github_token,required=false \
    TOKEN="$(cat /run/secrets/github_token 2>/dev/null || true)" && \
    if [ -n "$TOKEN" ]; then \
        GIT_CONFIG_COUNT=2 \
        GIT_CONFIG_KEY_0="url.https://${TOKEN}@${GITHUB_REWRITE_BASE}/.insteadOf" \
        GIT_CONFIG_VALUE_0=https://github.com/ \
        GIT_CONFIG_KEY_1=http.version \
        GIT_CONFIG_VALUE_1=HTTP/1.1 \
        go mod download; \
    else \
        go mod download; \
    fi
COPY src ./src
COPY main.go ./
# 从前端阶段拷贝构建产物
COPY --from=frontend /app/src/dist ./src/dist
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=0.0.1
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags "-X main.Version=${VERSION} -X main.BuildTime=$(date +%s) -s -w" \
    -o xcloud .

# ===== 阶段3: 最小运行镜像 =====
FROM alpine:3.20
ARG ALPINE_MIRROR
WORKDIR /app
RUN sed -i "s/dl-cdn.alpinelinux.org/${ALPINE_MIRROR}/g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/xcloud .
CMD ["./xcloud"]
