# GOSO — Makefile (Harness)

.PHONY: verify lint vet fmt fmt-fix test build doctor version help scan e2e smoke

help:
	@echo "Targets: verify | lint | vet | fmt | test | scan | e2e | build | smoke"

# AC-04 / US-01: vet + fmt + test + gitleaks/semgrep (if installed) + e2e
verify: vet fmt test scan e2e
	@echo "==> verify: OK"

lint: vet fmt scan

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l gateway)" || (echo "gofmt failed — run 'make fmt-fix'"; gofmt -l gateway; exit 1)

fmt-fix:
	gofmt -w gateway

test:
	go test ./... -count=1

scan:
	@sh scripts/scan.sh

e2e:
	@sh scripts/e2e.sh

build:
	go build -o bin/goso-gateway ./gateway/cmd/goso-gateway

smoke:
	go run ./gateway/cmd/goso-gateway version
	go run ./gateway/cmd/goso-gateway doctor

version:
	go run ./gateway/cmd/goso-gateway version

doctor:
	go run ./gateway/cmd/goso-gateway doctor
