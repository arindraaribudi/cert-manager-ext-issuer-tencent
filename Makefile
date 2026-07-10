# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# Get the currently used golang install path (i.e. where dependencies are installed), or default
GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(GOPATH)/bin
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line is non-zero, a variable is unset,
# or an error in a pipe.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.DEFAULT_GOAL := help

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_0-9-]+:.*?## / {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: manifests
manifests: controller-gen ## Generate CRD objects.
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:dir=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.12.2
.PHONY: golangci-lint
golangci-lint: ## Download golangci-lint locally if necessary.
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code.
	$(GOLANGCI_LINT) run --timeout=5m ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

.PHONY: ci
ci: lint test ## Run lint and tests for CI.

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

# PLATFORMS defines the target platforms for the manager image to be built for.
PLATFORMS ?= linux/arm64 linux/amd64 linux/s390x linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	- docker buildx create --name project-v3-builder
	docker buildx use project-v3-builder
	- docker buildx build --push --platform=$(PLATFORMS) --tag ${IMG} .

.PHONY: generate-install
generate-install: manifests kustomize ## Build install.yaml at repo root.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > install.yaml

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate consolidated install.yaml in dist/.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

.PHONY: deploy
deploy: manifests ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	kustomize build config/default | kubectl delete -f -

.PHONY: bundle
bundle: manifests ## Generate bundle artifacts.
	kustomize build config/manifests | operator-sdk generate bundle --overwrite --version $(VERSION)
	operator-sdk bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,v0.21.0)

ENVTEST = $(shell pwd)/bin/setup-envtest
.PHONY: envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,release-0.19)

ENVTEST_K8S_VERSION = 1.31.0

KUSTOMIZE = $(shell pwd)/bin/kustomize
.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

KUSTOMIZE_VERSION ?= v5.4.3

# go-install-tool will 'go install' any package with custom target and name of binary, if it is not found.
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
echo "Downloading $(2)@$(3)" ; \
GOBIN=$(shell pwd)/bin GOFLAGS=-mod=mod go install $(2)@$(3) ; \
mv "$$(echo "$(1)" | sed 's/$(3)$$//')" $(1)-$(3) ; \
}
@[ -f "$(1)" ] || mv "$(1)-$(3)" $(1)
endef
