module contacts

go 1.25.0

require (
	aicoded.dev/framework v0.0.0-00010101000000-000000000000
	github.com/go-sql-driver/mysql v1.10.1
	github.com/stretchr/testify v1.12.1
	google.golang.org/protobuf v1.36.12
)

require (
	connectrpc.com/connect v1.21.0 // indirect
	filippo.io/edwards25519 v1.2.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)

replace aicoded.dev/framework => ../..
