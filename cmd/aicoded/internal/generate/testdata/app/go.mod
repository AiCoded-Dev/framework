module example.com/app

go 1.25.0

require (
	aicoded.dev/framework v0.0.0-00010101000000-000000000000
	github.com/coder/websocket v1.8.15
	github.com/stretchr/testify v1.12.1
)

require (
	connectrpc.com/connect v1.21.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace aicoded.dev/framework => ../../../../../..
