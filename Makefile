fmt:
	go fmt ./...

vet: fmt
	go vet ./...

test: vet
	go test ./...

test-race: vet
	go test -race ./...

run:
	go run cmd/bot/main.go

