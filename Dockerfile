# API build stage
FROM golang:1.21.0-alpine3.17 AS go-builder
ARG GOPROXY=goproxy.cn

ENV GOPROXY=https://${GOPROXY},direct
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
RUN apk add --no-cache make bash git tzdata
WORKDIR /clickvisual

COPY go.mod go.sum ./
RUN go mod download -x
COPY . .
RUN go build -o ./bin/clickvisual


# Fianl running stage
FROM alpine:3.17
LABEL maintainer="clickvisual@shimo.im"

WORKDIR /clickvisual

COPY --from=go-builder /clickvisual/bin/clickvisual ./bin/
COPY --from=go-builder /clickvisual/config ./config

EXPOSE 9001
EXPOSE 9003
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
RUN apk --update add --no-cache tzdata

#CMD ["sh", "-c", "./bin/clickvisual server"]
