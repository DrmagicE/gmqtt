FROM golang:latest AS builder

ADD . /go/src/github.com/DrmagicE/gmqtt
WORKDIR /go/src/github.com/DrmagicE/gmqtt/cmd/gmqttd

ENV GO111MODULE on
ENV GOPROXY https://goproxy.cn

EXPOSE 1883
EXPOSE 8883
EXPOSE 8082
EXPOSE 8083
EXPOSE 8084

RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o gmqttd .

FROM alpine:3.12

WORKDIR /gmqttd
COPY --from=builder /go/src/github.com/DrmagicE/gmqtt/cmd/gmqttd/gmqttd .
ENV PATH=$PATH:/gmqttd
RUN chmod +x gmqttd
ENTRYPOINT ["gmqttd","start"]


