APP := officecli-platform

.PHONY: build test web-install web-build web-test dev fmt

build:
	go build -o bin/$(APP) ./cmd/platform

test:
	go test ./...

web-install:
	cd web/admin && npm install
	cd web/app && npm install
	cd web/site && npm install

web-build:
	cd web/admin && npm run build
	cd web/app && npm run build
	cd web/site && npm run build

web-test:
	cd web/admin && npm run test -- --run
	cd web/app && npm run test -- --run
	cd web/site && npm run test -- --run

dev:
	go run ./cmd/platform

fmt:
	gofmt -w $(shell find . -name '*.go' -not -path './web/*')
