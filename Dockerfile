# Dockerfile
# 阶段1: 构建
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /anime-tip ./cmd/server/

# 阶段2: 运行
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /anime-tip .
COPY web/ ./web/
ENV PORT=8080
ENV CHECK_INTERVAL="0 * * * *"
ENV DB_PATH=/data/anime-tip.db
EXPOSE 8080
VOLUME /data
CMD ["./anime-tip"]
