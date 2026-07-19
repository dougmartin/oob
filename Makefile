BINARY := oob
INSTALL_DIR ?= $(HOME)/.local/bin

.PHONY: build install init test vet fmt clean

build: ## Build the oob binary into the repo root
	go build -o $(BINARY) .

install: ## Build and install oob into $(INSTALL_DIR), then run init
	go build -o $(INSTALL_DIR)/$(BINARY) .
	$(INSTALL_DIR)/$(BINARY) init

init: build ## Run init using the freshly built local binary
	./$(BINARY) init

test: ## Run tests
	go test ./...

vet: ## go vet
	go vet ./...

fmt: ## Format
	gofmt -w .

clean: ## Remove built binary
	rm -f $(BINARY)
