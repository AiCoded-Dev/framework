module billing

go 1.26.0

require (
	aicoded.dev/framework v0.0.0-00010101000000-000000000000
	google.golang.org/protobuf v1.36.12
)

require connectrpc.com/connect v1.21.0 // indirect

replace aicoded.dev/framework => ../../../../../../..
