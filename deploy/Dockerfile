# syntax=docker/dockerfile:1.6
#
# 编译 rustdesk-api(含 CE-M0-4 / M1-1..7 / M1-9 改造)+ 用同级 rustdesk-api-web 编 admin 前端,
# 产出 thin runtime image。
# CI 由 .github/workflows/ce-publish.yml 调用,通过 BuildKit named context `api-web`
# 注入 estel-li/rustdesk-api-web 的源码,不在 build 期 git clone。
# 本地手动 build:
#   docker buildx build -f deploy/Dockerfile -t rustdesk-ce/api:dev \
#       --build-context api-web=../rustdesk-api-web .

ARG GO_VERSION=1.23
ARG NODE_VERSION=20

# ---- 阶段 A:Go 编译 apimain ----
FROM golang:${GO_VERSION}-bookworm AS builder

# CGO 必需(gorm sqlite 驱动)
RUN apt-get update && apt-get install -y --no-install-recommends \
        gcc \
        libc6-dev \
        libsqlite3-dev \
        git \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# release/apimain 静态二进制。docs/api/ docs/admin/ 已 commit,不重复 swag。
ENV CGO_ENABLED=1 GOOS=linux
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags "-s -w" -o /out/apimain ./cmd

# ---- 阶段 B:Node 编译 admin 前端 ----
FROM node:${NODE_VERSION}-bookworm-slim AS admin-web

WORKDIR /web
COPY --from=api-web . .

RUN --mount=type=cache,target=/root/.npm \
    npm ci

RUN --mount=type=cache,target=/root/.npm \
    NODE_ENV=production npm run build \
    && ls -la dist/

# ---- 阶段 C:runtime ----
FROM debian:bookworm-slim

# util-linux 自带 setpriv(entrypoint 降权用),bookworm-slim 默认带,无需额外装
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        tzdata \
        tini \
        adduser \
        curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r -g 1000 rustdesk \
    && useradd -r -u 1000 -g rustdesk -d /app -s /usr/sbin/nologin rustdesk \
    && mkdir -p /app/data /app/runtime \
    && chown -R rustdesk:rustdesk /app

WORKDIR /app

COPY --from=builder   /out/apimain     /app/apimain
COPY --from=builder   /src/conf        /app/conf
COPY --from=builder   /src/resources   /app/resources
COPY --from=builder   /src/docs        /app/docs
# admin 前端落到 /app/resources/admin,Gin 的 StaticFS("/_admin") 会从这里取
COPY --from=admin-web /web/dist        /app/resources/admin

# Entrypoint 脚本:启动时 chown /app/data /app/runtime + 降权到 rustdesk。
COPY deploy/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh \
    && chown -R rustdesk:rustdesk /app

# 注意:这里 *不* USER rustdesk —— 必须以 root 进入 entrypoint 才能 chown
# bind mount 目录(Linux),entrypoint 内部用 setpriv 降权到 rustdesk(UID 1000)。

# 21114 API/Web、21115 metrics(CE-M0-4,默认 loopback)
EXPOSE 21114 21115

VOLUME ["/app/data"]

# /api/version 是 Go 层固定路由,能同时验证 apimain 进程 + Gin router;
# 上游目前没有 /api/health,CE-M0-4 metrics 落地后可换回更专门的路由。
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -fsS http://127.0.0.1:21114/api/version || exit 1

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/docker-entrypoint.sh", "/app/apimain"]
