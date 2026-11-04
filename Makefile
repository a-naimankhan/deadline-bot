fmt:
	go fmt ./...

vet: fmt
	CGO_ENABLED=0 go vet ./...

test: vet
	CGO_ENABLED=0 go test ./...

test-race:
	CGO_ENABLED=1 go test -race ./...

run:
	go run cmd/bot/main.go

docker-build:
	docker build -t deadline-bot:dev .
