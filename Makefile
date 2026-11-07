.PHONY: run test race vet build lint tidy clean

run:
	go run ./cmd/server

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

build:
	go build -trimpath -o bin/server ./cmd/server

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

clean:
	rm -rf bin
