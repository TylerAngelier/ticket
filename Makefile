BIN_DIR := build
PREFIX ?= $(HOME)/.local
BINS := ticket-ls ticket-list ticket-ready ticket-blocked ticket-dep ticket-closed ticket-show

.PHONY: build test unit behave differential bench install uninstall clean

build: $(BIN_DIR)/ticket-ls $(addprefix $(BIN_DIR)/,ticket-list ticket-ready ticket-blocked)

$(BIN_DIR)/ticket-ls:
	go build -trimpath -o $@ ./cmd/ticket-ls

$(BIN_DIR)/ticket-%: $(BIN_DIR)/ticket-ls
	ln -sf ticket-ls $@

unit:
	go vet ./...
	go test ./...

behave:
	uv run --with behave behave

test: build unit behave

differential: build
	./scripts/differential-check.sh 500

bench: build
	./scripts/bench.sh 5000

install: build
	mkdir -p "$(PREFIX)/bin"
	ln -sf "$(CURDIR)/ticket" "$(PREFIX)/bin/tk"
	ln -sf "$(CURDIR)/ticket" "$(PREFIX)/bin/ticket"
	for b in $(BINS); do \
		ln -sf "$(CURDIR)/$(BIN_DIR)/ticket-ls" "$(PREFIX)/bin/$$b"; \
	done
	@echo "Installed tk + plugins to $(PREFIX)/bin"

uninstall:
	rm -f "$(PREFIX)/bin/tk" "$(PREFIX)/bin/ticket"
	for b in $(BINS); do rm -f "$(PREFIX)/bin/$$b"; done

clean:
	rm -rf $(BIN_DIR)
