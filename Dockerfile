# Build 阶段：在 golang 官方镜像中编译，不写死 CPU 架构，兼容 linux/arm64 与 linux/amd64。
FROM --platform=$BUILDPLATFORM golang:1.22 AS build

WORKDIR /src
# 先复制 go.mod 以便利用依赖缓存。
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/server ./cmd/server

# 运行阶段：使用精简的 distroless 风格静态二进制运行。
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
