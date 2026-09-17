package main

import (
	"os"

	"github.com/xhelodev/tg/internal/cli"
)

// Inyectados vía -ldflags "-X main.version=... -X main.commit=..."
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.VersionInfo{
		Version: version,
		Commit:  commit,
	}))
}
