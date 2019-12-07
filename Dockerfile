# Build Stage
FROM golang:latest

ADD . /go/src/github.com/DrmagicE/gmqtt
WORKDIR /go/src/github.com/DrmagicE/gmqtt/cmd/broker


ENV GO111MODULE on
ENV GOPROXY https://mirrors.aliyun.com/goproxy/


EXPOSE 1883

RUN go build .
CMD ["./broker"]


