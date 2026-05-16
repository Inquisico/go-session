.PHONY: all update lint run build build-in-docker install docker

all: update run

update:
	@go mod tidy \
		&& go mod vendor

lint:
	@golangci-lint run ./...

test:
	@go test ./... -coverprofile=tmp/coverage.out

coverage:
	@go tool cover -html=tmp/coverage.out
