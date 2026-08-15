BINARY := unlockscope
VERSION ?= $(shell tr -d '\n' < VERSION)

.PHONY: all fmt test race vet lint build clean
all: test vet build

fmt:
	gofmt -w cmd/unlockscope internal/model internal/probe internal/provider

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

lint: fmt vet
	@test -z "$$(gofmt -l cmd internal)" || (echo "gofmt reported files"; exit 1)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/unlockscope

clean:
	rm -f $(BINARY)
