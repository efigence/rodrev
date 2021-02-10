# generate version number
version=$(shell git describe --tags --long --always --dirty|sed 's/^v//')
.PHONY: release
all:
	go build -ldflags "-X main.version=$(version)" -o rv cmd/rv/*.go
	go build -ldflags "-X main.version=$(version)" -o rvd cmd/rvd/*.go
	-@go fmt
release:
	mkdir -p release
	GOARCH=amd64 go build  -ldflags "-X main.version=$(version)" -o release/rv.amd64 cmd/rv/*.go
	GOARCH=amd64 go build  -ldflags "-X main.version=$(version)" -o release/rvd.amd64 cmd/rvd/*.go
	GOARCH=arm64 go build  -ldflags "-X main.version=$(version)" -o release/rv.arm64 cmd/rv/*.go
	GOARCH=arm64 go build  -ldflags "-X main.version=$(version)" -o release/rvd.arm64 cmd/rvd/*.go
	GOARCH=386 go build  -ldflags "-X main.version=$(version)" -o release/rv.i386 cmd/rv/*.go
	GOARCH=386 go build  -ldflags "-X main.version=$(version)" -o release/rvd.i386 cmd/rvd/*.go
	cp scripts/fence_rvd.pl release/fence_rvd
version:
	@echo $(version)
