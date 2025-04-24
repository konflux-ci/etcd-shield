GO := go

IMAGE_BUILDER := podman
IMG := etcd-shield:latest

build:
	$(GO) build ./cmd/etcd-shield/main.go

build-image:
	$(IMAGE_BUILDER) build -t $(IMG) .

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-coverage:
	$(GO) test -covermode=atomic -coverprofile=cover.out ./...

lint-yaml:
	@yamllint ./

lint-go:
	@$(GO) run \
		-modfile $(shell realpath ./hack/tools/golang-ci/go.mod) \
		github.com/golangci/golangci-lint/v2/cmd/golangci-lint \
		run

lint: lint-go lint-yaml
