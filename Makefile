.DEFAULT_GOAL := help
.PHONY: help setup generate check check-generated check-native check-author dev-web dev-desktop build-runtime build-cli package-desktop stage-web
help:
	@echo 'setup generate check check-generated check-native check-author dev-web dev-desktop build-runtime build-cli package-desktop'
# Runtime fixture sockets must fit the Unix address limit on macOS.
check check-generated check-native check-author: export TMPDIR := /tmp
setup generate check check-generated check-native check-author dev-web dev-desktop build-runtime build-cli package-desktop stage-web:
	node tooling/workspace.mjs $@
