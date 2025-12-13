.PHONY: build run clean tidy

build:
	go build -o caddy ./src

run: build
	bash -c "[ -f .env ] && set -a && source .env && set +a && ./caddy run --config Caddyfile"

clean:
	rm -f caddy

tidy:
	go mod tidy
