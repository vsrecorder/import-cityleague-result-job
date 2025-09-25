.PHONY: build
build:
	go mod download && go mod tidy
	go build -o bin/enqueue cmd/enqueue/main.go
	go build -o bin/dequeue cmd/dequeue/main.go