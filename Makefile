.PHONY: fmt test vet build check

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

test:
	go test ./...

vet:
	go vet ./...

build:
	mkdir -p dist
	go build -o dist/seedflux ./cmd/seedflux

check: fmt test vet build
