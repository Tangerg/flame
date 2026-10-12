BIN := build/flame

.PHONY: build run

build:
	node ../plugins/build.mjs --os "$$(go env GOOS)" --arch "$$(go env GOARCH)" --output "$(abspath build/plugins)" --replace
	go build -o "$(BIN)" .

run: build
	./$(BIN)
