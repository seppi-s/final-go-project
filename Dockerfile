FROM golang:1.24-alpine AS builder
RUN apk add --no-cache build-base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go main_test.go ./
COPY public ./public
RUN go test ./... && CGO_ENABLED=1 go build -tags netgo,osusergo -trimpath -ldflags="-s -w -linkmode external -extldflags '-static'" -o /out/student-console .

FROM alpine:3.21 AS runtime
RUN addgroup -g 10001 app && adduser -D -u 10001 -G app app && mkdir /data && chown app:app /data
WORKDIR /app
ENV DATA_DIR=/data
COPY --from=builder /out/student-console /app/student-console
COPY --chown=app:app public ./public
USER app
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD ["/app/student-console", "healthcheck"]
CMD ["/app/student-console", "serve"]
