protoc -I. \
-I/home/charlie/go/pkg/mod/github.com/grpc-ecosystem/grpc-gateway@v1.16.0 \
-I/home/charlie/go/pkg/mod/github.com/grpc-ecosystem/grpc-gateway@v1.16.0/third_party/googleapis \
--go-grpc_out=../ \
--go_out=../ \
--grpc-gateway_out=../ \
--swagger_out=../swagger \
*.proto
