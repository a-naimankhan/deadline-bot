fmt:
	go fmt ./...

vet: fmt
	go vet ./...

test: vet
	go test ./...

test-race:
	CGO_ENABLED=1 go test -race ./...

run:
	go run cmd/bot/main.go

