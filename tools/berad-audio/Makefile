GO ?= go
PYTHON ?= python3
SDK_ROOT := ../..
BIN_DIR := $(SDK_ROOT)/.cache/bin
EXE := $(if $(filter Windows_NT,$(OS)),.exe,)

.PHONY: build audio-probe plan validate quality-gate provision clean help
build: audio-probe

audio-probe:
	@mkdir -p "$(BIN_DIR)"
	$(GO) build -o "$(BIN_DIR)/audio-probe$(EXE)" ./cmd/audio-probe

plan validate quality-gate provision:
	$(PYTHON) -I -B $(SDK_ROOT)/scripts/check.py $@ --module tools/berad-audio

clean:
	rm -f "$(BIN_DIR)/audio-probe" "$(BIN_DIR)/audio-probe.exe"

help:
	@printf '%s\n' 'build/audio-probe: build standalone source probe' 'plan/validate/quality-gate: SDK-owned selected checks' 'provision: explicit network dependency setup' 'clean: remove tool binaries only'
