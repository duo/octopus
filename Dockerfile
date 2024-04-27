FROM golang:1.22-alpine AS builder

RUN apk add --no-cache --update --quiet --no-progress build-base

WORKDIR /build

COPY ./ .

RUN set -ex \
    && cd /build \
    && go build -o octopus

FROM alpine:latest

RUN apk add --no-cache --update --quiet --no-progress ffmpeg tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone
#&& apk del --quiet --no-progress tzdata

COPY --from=builder /build/octopus /usr/bin/octopus
RUN chmod +x /usr/bin/octopus

WORKDIR /data

ENTRYPOINT [ "/usr/bin/octopus" ]
