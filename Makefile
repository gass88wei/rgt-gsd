.PHONY: test build lint clean

test:
	go test ./... -v -count=1

build:
	go build -o bin/rgt-gsd ./cmd/rgt-gsd/

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/
