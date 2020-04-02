# generate version number
version=$(shell git describe --tags --long --always --dirty|sed 's/^v//')

all:
	go build -ldflags "-X main.version=$(version)" -o rv cmd/rv/*.go
	go build -ldflags "-X main.version=$(version)" -o rvd cmd/rvd/*.go
	-@go fmt

static:
	go build -ldflags "-X main.version=$(version) -extldflags \"-static\"" -o rv.static cmd/rv/*.go
	go build -ldflags "-X main.version=$(version) -extldflags \"-static\"" -o rvd.static rvd.go

arm:
	GOARCH=arm go build  -ldflags "-X main.version=$(version) -extldflags \"-static\"" -o rv.arm cmd/rv/*.go
	GOARCH=arm go build  -ldflags "-X main.version=$(version) -extldflags \"-static\"" -o rvd.arm cmd/rvd/rvd/rvd.go
	GOARCH=arm64 go build  -ldflags "-X main.version=$(version) -extldflags \"-static\"" -o rv.arm64 cmd/rv/*.go
	GOARCH=arm64 go build  -ldflags "-X main.version=$(version) -extldflags \"-static\"" -o rvd.arm64 cmd/rvd/*.go
version:
	@echo $(version)
