# Build context = repository root:  docker build -f backend/Dockerfile .
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/server /app/server
COPY backend/web /app/web
COPY website /app/website
RUN mkdir -p /app/data && chown app /app/data
USER app
ENV PORT=8080 ADMIN_DIR=/app/web/admin SITE_DIR=/app/website DB_DSN=/app/data/taxi.db TZ=Asia/Ashgabat
EXPOSE 8080
ENTRYPOINT ["/app/server"]
