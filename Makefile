.PHONY: build test test-integration vet all

all: vet test build

build:
	go build -o bootenv .

test:
	go test ./...

# Real-btrfs integration tests. Requires root + btrfs-progs; individual tests
# skip themselves when prerequisites are missing.
test-integration:
	sudo -E go test -tags=integration ./...

vet:
	go vet ./...
	go vet -tags=integration ./...
