FROM golang:latest

ADD . /go/src/github.com/DrmagicE/gmqtt
WORKDIR /go/src/github.com/DrmagicE/gmqtt/cmd/broker

ENV GO111MODULE on
#ENV GOPROXY https://goproxy.cn

EXPOSE 1883
EXPOSE 8081
EXPOSE 8082

RUN go build .
CMD ["./broker"]


