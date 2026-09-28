BIN := bin/swingdesk
export CGO_ENABLED := 0

.PHONY: build run test vet clean

build:
	go build -o $(BIN) ./cmd/swingdesk

run:
	go run ./cmd/swingdesk

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf bin
