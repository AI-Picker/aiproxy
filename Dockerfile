FROM golang:1.23-alpine AS builder

ARG VERSION

WORKDIR /aiproxy

COPY ./ ./

RUN go build -trimpath -tags "jsoniter" -ldflags "-s -w" -o aiproxy

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata ffmpeg curl && \
    rm -rf /var/cache/apk/*

COPY --from=builder /aiproxy/aiproxy /usr/local/bin/aiproxy

ENV PUID=0 PGID=0 UMASK=022

ENV FFPROBE_ENABLED=true

EXPOSE 3000

ENTRYPOINT ["aiproxy"]
