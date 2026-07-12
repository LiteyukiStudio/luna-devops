# 构建前端静态资源，供 API 镜像在 embed_web 模式下内嵌 SPA。
FROM node:25-alpine AS web-build

WORKDIR /src/web

# 固定 pnpm 版本，避免不同构建环境解析 lockfile 时行为漂移。
RUN npm install -g pnpm@10.20.0

# 先复制依赖清单以复用 Docker layer cache，再复制完整前端源码。
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

# 准备 Go 源码和依赖缓存，后续普通构建与内嵌前端构建共用该阶段。
FROM golang:1.26.5-alpine AS source

WORKDIR /src

# 先下载 Go module 依赖，减少业务代码变更导致的重复下载。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 构建指定命令入口，默认 TARGET=api；worker 可通过 build-arg 复用。
FROM source AS build

ARG TARGET=api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${TARGET}

# 构建内嵌前端静态资源的 API 二进制，用于完整平台部署镜像。
FROM source AS build-embed-web

COPY --from=web-build /src/web/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -tags=embed_web -ldflags="-s -w" -o /out/app ./cmd/api

# API 完整部署运行镜像：包含 embed_web 构建产物，可直接提供前端 SPA。
FROM alpine:3.22 AS runtime-embed-web

# 安装运行期所需证书、git 和 docker CLI，并创建非 root 用户运行应用。
RUN apk add --no-cache ca-certificates docker-cli git && addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=build-embed-web /out/app /app/app

USER app
EXPOSE 8080

ENTRYPOINT ["/app/app"]

# 普通运行镜像：用于 api / worker 等不需要内嵌前端的目标。
FROM alpine:3.22 AS runtime

# 保持与 embed_web runtime 相同的基础运行环境，降低不同镜像目标的差异。
RUN apk add --no-cache ca-certificates docker-cli git && addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=build /out/app /app/app

USER app
EXPOSE 8080

ENTRYPOINT ["/app/app"]

# Gateway Traffic Probe 运行镜像：只需要证书和独立 9090 健康/指标端口。
FROM alpine:3.22 AS runtime-probe

RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=build /out/app /app/app

USER app
EXPOSE 9090

ENTRYPOINT ["/app/app"]
