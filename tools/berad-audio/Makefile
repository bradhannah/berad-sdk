GO ?= go
PYTHON ?= python3
SDK_ROOT := ../..
BIN_DIR := $(SDK_ROOT)/.cache/bin
EXE := $(if $(filter Windows_NT,$(OS)),.exe,)

.PHONY: build audio-editor audio-catalog audio-probe audio-runtime-probe plan validate quality-gate provision clean help
build: audio-editor audio-catalog audio-probe

audio-editor:
	@mkdir -p "$(BIN_DIR)"
	$(GO) build -o "$(BIN_DIR)/audio-editor$(EXE)" ./cmd/audio-editor

audio-probe:
	@mkdir -p "$(BIN_DIR)"
	$(GO) build -o "$(BIN_DIR)/audio-probe$(EXE)" ./cmd/audio-probe

audio-catalog:
	@mkdir -p "$(BIN_DIR)"
	$(GO) build -o "$(BIN_DIR)/audio-catalog$(EXE)" ./cmd/audio-catalog

# Optional graphical-loop probe, intentionally excluded from the terminal build.
audio-runtime-probe:
	@mkdir -p "$(BIN_DIR)"
	$(GO) build -o "$(BIN_DIR)/audio-runtime-probe$(EXE)" ./cmd/audio-runtime-probe

plan validate quality-gate provision:
	$(PYTHON) -I -B $(SDK_ROOT)/scripts/check.py $@ --module tools/berad-audio

clean:
	rm -f "$(BIN_DIR)/audio-editor" "$(BIN_DIR)/audio-editor.exe"
	rm -f "$(BIN_DIR)/audio-catalog" "$(BIN_DIR)/audio-catalog.exe"
	rm -f "$(BIN_DIR)/audio-probe" "$(BIN_DIR)/audio-probe.exe"
	rm -f "$(BIN_DIR)/audio-runtime-probe" "$(BIN_DIR)/audio-runtime-probe.exe"

help:
	@printf '%s\n' 'build: build editor, catalog and standalone source probe' 'audio-editor/audio-catalog/audio-probe: individual builds' 'plan/validate/quality-gate: SDK-owned selected checks' 'provision: explicit network dependency setup' 'clean: remove tool binaries only'
