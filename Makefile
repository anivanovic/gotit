PACKAGE=github.com/anivanovic/gotit
VERSION=$(shell git describe --tags --always --abbrev=0 --match='v[0-9]*.[0-9]*.[0-9]*' 2> /dev/null | sed 's/^.//')
COMMIT_HASH=$(shell git rev-parse HEAD)
BUILD_TIMESTAMP=$(shell date '+%Y-%m-%dT%H:%M:%S')

LDFLAGS="-X $(PACKAGE)/pkg/command.Version=$(VERSION) -X $(PACKAGE)/pkg/command.Commit=$(COMMIT_HASH) -X $(PACKAGE)/pkg/command.Build=$(BUILD_TIMESTAMP)"

clean:
	rm -rf bin/*

build:
	go build -ldflags=$(LDFLAGS) -o bin/gotit github.com/anivanovic/gotit/cmd/gotit

test:
	go test --cover ./...

run:
	go run ./cmd/gotit

.PHONY: clean run 