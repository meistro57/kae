.PHONY: all build lens kae run-lens run-kae deps tidy clean fmt vet

# ── Config ──
BINARY_KAE  = ./bin/kae
BINARY_LENS = ./bin/lens
CONFIG_LENS = config/lens.yaml
CONFIG_KAE  = config/kae.yaml

# ── Build ──
all: build

build: kae lens

kae:
	@echo "→ building KAE..."
	@mkdir -p bin
	go build -o $(BINARY_KAE) ./cmd/kae

lens:
	@echo "→ building KAE Lens..."
	@mkdir -p bin
	go build -o $(BINARY_LENS) ./cmd/lens

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

clean:
	rm -rf bin/

# ── Help ──
help:
	@echo ""
	@echo "KAE + KAE Lens — Make targets"
	@echo "──────────────────────────────"
	@echo "  make build         Build both binaries to ./bin/"
	@echo "  make run-lens      Run KAE Lens (TUI + web dashboard)"
	@echo "  make run-lens-notui  Run Lens without TUI (plain logs)"
	@echo "  make run-kae       Run KAE ingestion engine"
	@echo "  make qdrant-up     Start Qdrant in Docker"
	@echo "  make qdrant-down   Stop Qdrant Docker container"
	@echo "  make deps          Download Go dependencies"
	@echo "  make tidy          Run go mod tidy"
	@echo "  make fmt           Run go fmt"
	@echo "  make vet           Run go vet"
	@echo "  make clean         Remove built binaries"
	@echo ""
