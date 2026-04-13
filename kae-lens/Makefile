.PHONY: all build lens kae mcp analyzer run-lens run-kae deps tidy clean fmt vet test healthcheck component-test

# ── Config ──
BINARY_KAE  = ../kae
BINARY_LENS = ./lens
CONFIG_LENS = config/lens.yaml
CONFIG_KAE  = config/kae.yaml

# ── Build ──
all: build

build: kae lens mcp analyzer

kae:
	@echo "→ building KAE..."
	@cd .. && go build -o kae .

lens:
	@echo "→ building KAE Lens..."
	go build -o lens ./cmd/lens

mcp:
	@echo "→ building KAE MCP server..."
	@cd ../mcp && go build -o kae_mcp .

analyzer:
	@echo "→ building KAE Analyzer..."
	@cd ../kae-analyzer && go build -o kae-analyzer .

# ── Run ──
run-lens:
	@echo "→ starting KAE Lens..."
	$(BINARY_LENS) --config $(CONFIG_LENS)

run-lens-notui:
	@echo "→ starting KAE Lens (no TUI)..."
	$(BINARY_LENS) --config $(CONFIG_LENS) --no-tui

run-kae:
	@echo "→ starting KAE ingestion..."
	$(BINARY_KAE) --config $(CONFIG_KAE)

# ── Qdrant (Docker) ──
qdrant-up:
	@echo "→ starting Qdrant via Docker..."
	docker run -d --name kae-qdrant \
		-p 6333:6333 \
		-p 6334:6334 \
		-v $(PWD)/qdrant_storage:/qdrant/storage:z \
		qdrant/qdrant
	@echo "✓ Qdrant running at http://localhost:6333/dashboard"

qdrant-down:
	docker stop kae-qdrant && docker rm kae-qdrant

qdrant-logs:
	docker logs -f kae-qdrant

# ── Dev ──
deps:
	go mod download

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	@echo "→ running tests..."
	go test ./internal/lens/... -v

healthcheck:
	@echo "→ running health check..."
	@chmod +x healthcheck.sh
	./healthcheck.sh

component-test:
	@echo "→ running component integration test..."
	@cd test && go run integration.go

clean:
	rm -f lens
	@cd .. && rm -f kae
	@cd ../mcp && rm -f kae_mcp
	@cd ../kae-analyzer && rm -f kae-analyzer

# ── Help ──
help:
	@echo ""
	@echo "KAE + KAE Lens — Make targets"
	@echo "──────────────────────────────"
	@echo "  make build           Build both binaries to ./bin/"
	@echo "  make test            Run unit tests"
	@echo "  make healthcheck     Run full system health check"
	@echo "  make component-test  Run component integration test"
	@echo "  make run-lens        Run KAE Lens (TUI + web dashboard)"
	@echo "  make run-lens-notui  Run Lens without TUI (plain logs)"
	@echo "  make run-kae         Run KAE ingestion engine"
	@echo "  make qdrant-up       Start Qdrant in Docker"
	@echo "  make qdrant-down     Stop Qdrant Docker container"
	@echo "  make deps            Download Go dependencies"
	@echo "  make tidy            Run go mod tidy"
	@echo "  make fmt             Run go fmt"
	@echo "  make vet             Run go vet"
	@echo "  make clean           Remove built binaries"
	@echo ""
