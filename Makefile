.PHONY: build build-go install-python test test-go test-python clean

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")

build: build-go install-python

build-go:
	cd db-gateway && CGO_ENABLED=0 go build \
		-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" \
		-o db-gateway .

install-python:
	printf '__commit__ = "%s"\n' "$(COMMIT)" > cli/yamlq/_commit.py
	cd cli && uv pip install -e ".[dev,tui]"

test: test-go test-python

test-go:
	cd db-gateway && go test ./... -timeout 60s

test-python:
	cd cli && .venv/bin/python -m pytest tests/ -v --timeout=120

clean:
	rm -f db-gateway/db-gateway
	rm -f cli/yamlq/_commit.py
	find . -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null || true
