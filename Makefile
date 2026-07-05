PROVIDER      := pulumi-resource-exedev
VERSION       ?= 0.2.0
PKG           := exedev
PROVIDER_PATH := provider
BIN           := bin/$(PROVIDER)
LDFLAGS       := -X main.Version=$(VERSION)

.PHONY: build schema sdk sdk_go sdk_nodejs install test tidy clean

## build the provider plugin binary
build:
	cd $(PROVIDER_PATH) && go build -ldflags "$(LDFLAGS)" -o ../$(BIN) ./cmd/$(PROVIDER)

SCHEMA := provider/cmd/$(PROVIDER)/schema.json

## emit the Pulumi schema (committed for the registry listing)
schema: build
	pulumi package get-schema ./$(BIN) > $(SCHEMA)

## generate all SDKs
sdk: sdk_go sdk_nodejs

GO_SDK_MOD := github.com/firstatlast/pulumi-exedev/sdk/go

sdk_go: build
	rm -rf sdk/go
	pulumi package gen-sdk ./$(BIN) --language go --out sdk
	@# gen-sdk does not emit a go.mod for the Go SDK; create and tidy one
	cd sdk/go && go mod init $(GO_SDK_MOD) && \
		go get github.com/pulumi/pulumi/sdk/v3@latest && go mod tidy

sdk_nodejs: build
	rm -rf sdk/nodejs
	pulumi package gen-sdk ./$(BIN) --language nodejs --out sdk
	@# gen-sdk emits codegen-default deps; pin to current latest
	cd sdk/nodejs && npm pkg set dependencies.@pulumi/pulumi="^3.250.0" \
		devDependencies.typescript="^6.0.0" devDependencies.@types/node="^26"

## install the plugin locally for `pulumi up` testing
install: build
	pulumi plugin install resource $(PKG) $(VERSION) --file ./$(BIN)

## run unit tests
test:
	cd $(PROVIDER_PATH) && go test ./...

tidy:
	cd $(PROVIDER_PATH) && go mod tidy

clean:
	rm -rf bin dist
