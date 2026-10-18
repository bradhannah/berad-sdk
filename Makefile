.PHONY: plan validate quality-gate

plan:
	python3 -I -B scripts/check.py plan

validate:
	python3 -I -B scripts/check.py validate

quality-gate:
	python3 -I -B scripts/check.py quality-gate
