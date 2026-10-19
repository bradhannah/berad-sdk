PYTHON ?= python3
MODULE ?=
SELECT_MODULE = $(if $(MODULE),--module $(MODULE),)

.PHONY: plan validate quality-gate provision audio-runtime-check audio-tools-check audio-tools clean

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

# Only SDK-owned generated executables; retain validation proof and authored files.
clean:
	$(MAKE) -C tools/berad-audio clean
	# Remove executables left by the previous repository-cache output layout too.
	rm -f .cache/bin/audio-editor .cache/bin/audio-editor.exe
	rm -f .cache/bin/audio-catalog .cache/bin/audio-catalog.exe
	rm -f .cache/bin/audio-probe .cache/bin/audio-probe.exe
	rm -f .cache/bin/audio-runtime-probe .cache/bin/audio-runtime-probe.exe
