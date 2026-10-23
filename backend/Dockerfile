# Build context = repository root:  docker build -f backend/Dockerfile .
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
# timetzdata embeds the zone database (Asia/Ashgabat) so the runtime image needs no tzdata package
RUN CGO_ENABLED=0 go build -tags timetzdata -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
# CA roots for outgoing HTTPS (Firebase, bank, OpenStreetMap) come from the build stage: no package download here
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/server /app/server
COPY backend/web /app/web
COPY website /app/website
RUN mkdir -p /app/data && chown app /app/data
USER app
ENV PORT=8080 ADMIN_DIR=/app/web/admin SITE_DIR=/app/website DB_DSN=/app/data/taxi.db TZ=Asia/Ashgabat
EXPOSE 8080
ENTRYPOINT ["/app/server"]
