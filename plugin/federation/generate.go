package federation

//go:generate mockgen -source=./federation.pb.go -destination=./federation.pb_mock.go -package=federation
//go:generate mockgen -source=./federation_grpc.pb.go -destination=./federation_grpc.pb_mock.go -package=federation
//go:generate mockgen -source=./peer.go -destination=./peer_mock.go -package=federation
//go:generate mockgen -source=./membership.go -destination=./membership_mock.go -package=federation
