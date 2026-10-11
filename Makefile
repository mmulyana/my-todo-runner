BIN := dist/my-todo-runner

.PHONY: build test

build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o $(BIN) ./cmd/runner

test:
	go vet ./... && go test ./...
