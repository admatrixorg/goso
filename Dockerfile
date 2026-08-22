# syntax=docker/dockerfile:1
# GOSO Gateway — multi-stage, pure Go (no CGO). SPEC 012.

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY gateway ./gateway
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/goso-gateway ./gateway/cmd/goso-gateway

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
	&& adduser -D -u 1000 goso \
	&& mkdir -p /data && chown goso:goso /data
COPY --from=build /out/goso-gateway /usr/local/bin/goso-gateway
USER goso
WORKDIR /data
VOLUME /data
EXPOSE 8080
ENV GOSO_PORT=8080 \
	GOSO_HOST=0.0.0.0 \
	GOSO_DB_PATH=/data/goso.db \
	GOSO_ENV=development
HEALTHCHECK --interval=15s --timeout=3s --start-period=8s --retries=3 \
	CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["goso-gateway"]
CMD ["gateway"]
