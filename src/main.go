package main

import (
	caddycmd "github.com/caddyserver/caddy/v2/cmd"

	_ "github.com/caddyserver/caddy/v2/modules/standard"
	_ "github.com/neutrome-labs/open-ai-router-v2/src/modules"
)

func main() {
	caddycmd.Main()
}
