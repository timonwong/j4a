.PHONY: build test test-race vet fmt fmt-check check clean

build:
	go build -o bin/jiro ./cmd/jiro

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"

check: fmt-check vet test-race build

clean:
	rm -rf bin dist coverage.out
