.DEFAULT_GOAL := help
COMMANDS := setup generate check check-generated check-native check-author dev-web dev-desktop build-runtime build-cli package-desktop
.PHONY: help $(COMMANDS) stage-web
help:
	@echo '$(COMMANDS)'
# Runtime fixture sockets must fit the Unix address limit on macOS.
check check-generated check-native check-author: export TMPDIR := /tmp
$(COMMANDS) stage-web:
	node tooling/workspace.mjs $@
