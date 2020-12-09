protoc -I`pwd` \
-I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway \
-I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
--go-grpc_out=`pwd` \
--go_out=`pwd` \
--grpc-gateway_out=`pwd` \
--swagger_out=./swagger \
protos/*
