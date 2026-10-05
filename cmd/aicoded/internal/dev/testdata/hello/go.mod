module hello

go 1.25.0

require aicoded.dev/framework v0.0.0-00010101000000-000000000000

require (
	connectrpc.com/connect v1.21.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace aicoded.dev/framework => ../../../../../..
