PYTHON ?= $(if $(wildcard .cache/python-quality/bin/python),.cache/python-quality/bin/python,$(if $(wildcard .cache/python-quality/Scripts/python.exe),.cache/python-quality/Scripts/python.exe,python3))
GO ?= go
PYTHON_BOOTSTRAP ?= $(PYTHON)
MODULE ?=
SELECT_MODULE = $(if $(MODULE),--module $(MODULE),)
TEXT_DEMO_BIN = .cache/bin/berad-text-demo$(if $(filter Windows_NT,$(OS)),.exe,)
GRAPHICS_DEMO_BIN = .cache/bin/berad-graphics-demo$(if $(filter Windows_NT,$(OS)),.exe,)
.DEFAULT_GOAL := plan

.PHONY: python-tools python-format python-format-check python-lint python-check
python-tools:
	$(PYTHON_BOOTSTRAP) -I -B scripts/python_quality.py $@

python-format python-format-check python-lint python-check:
	$(PYTHON) -I -B scripts/python_quality.py $@

.PHONY: plan validate quality-gate provision audio-runtime-check audio-tools-check audio-tools clean
.PHONY: text-check build-text-demo text-demo text-demo-smoke
.PHONY: graphics-check
.PHONY: rng-check input-check
.PHONY: build-graphics-demo graphics-demo graphics-demo-smoke

graphics-check:
	$(MAKE) quality-gate MODULE=modules/berad-graphics

rng-check:
	$(MAKE) quality-gate MODULE=modules/berad-rng

input-check:
	$(MAKE) quality-gate MODULE=modules/berad-input

build-graphics-demo:
	mkdir -p .cache/bin
	GOWORK=off GOPROXY=off GOSUMDB=off $(GO) -C modules/berad-graphics build -mod=readonly -o "../../$(GRAPHICS_DEMO_BIN)" ./examples/atelier

graphics-demo: build-graphics-demo
	"./$(GRAPHICS_DEMO_BIN)"

graphics-demo-smoke: build-graphics-demo
	$(PYTHON) -I -B scripts/text_demo_smoke.py graphics
.PHONY: tools format format-check lint lint-all lint-fix lint-config-check
.PHONY: tooling-test

tooling-test:
	$(PYTHON) -I -B scripts/check.py tooling-test

tools:
	$(PYTHON) -I -B scripts/check.py tools

format:
	$(PYTHON) -I -B scripts/check.py format $(SELECT_MODULE)

format-check:
	$(PYTHON) -I -B scripts/check.py format-check $(SELECT_MODULE)

lint: lint-config-check
	$(PYTHON) -I -B scripts/check.py lint $(SELECT_MODULE)

lint-all: lint

lint-fix: lint-config-check
	$(PYTHON) -I -B scripts/check.py lint-fix $(SELECT_MODULE)

lint-config-check:
	$(PYTHON) -I -B scripts/check.py lint-config-check

plan:
	$(PYTHON) -I -B scripts/check.py plan $(SELECT_MODULE)

validate:
	$(PYTHON) -I -B scripts/check.py validate $(SELECT_MODULE)

quality-gate:
	$(PYTHON) -I -B scripts/check.py quality-gate $(SELECT_MODULE)

provision:
	$(PYTHON) -I -B scripts/check.py provision $(SELECT_MODULE)

audio-runtime-check:
	$(PYTHON) -I -B scripts/check.py quality-gate --module modules/berad-audio

audio-tools-check:
	$(PYTHON) -I -B scripts/check.py quality-gate --module tools/berad-audio

audio-tools:
	$(MAKE) -C tools/berad-audio build

text-check:
	$(MAKE) quality-gate MODULE=modules/berad-text

build-text-demo:
	mkdir -p .cache/bin
	GOWORK=off GOPROXY=off GOSUMDB=off $(GO) -C modules/berad-text build -mod=readonly -o "../../$(TEXT_DEMO_BIN)" ./examples/panel

text-demo: build-text-demo
	"./$(TEXT_DEMO_BIN)"

text-demo-smoke: build-text-demo
	$(PYTHON) -I -B scripts/text_demo_smoke.py

# Only SDK-owned generated executables; retain validation proof and authored files.
clean:
	$(MAKE) -C tools/berad-audio clean
	# Remove executables left by the previous repository-cache output layout too.
	rm -f .cache/bin/audio-editor .cache/bin/audio-editor.exe
	rm -f .cache/bin/audio-catalog .cache/bin/audio-catalog.exe
	rm -f .cache/bin/audio-probe .cache/bin/audio-probe.exe
	rm -f .cache/bin/audio-runtime-probe .cache/bin/audio-runtime-probe.exe
	rm -f .cache/bin/berad-text-demo .cache/bin/berad-text-demo.exe
	rm -f .cache/bin/berad-graphics-demo .cache/bin/berad-graphics-demo.exe
