package main

import (
	"flag"
	"log/slog"
	"os"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "../configs/controlplane/config.yaml", "path to config file")
	flag.Parse()

	if err := run(configPath); err != nil {
		slog.Error("controlplane exited", "error", err)
		os.Exit(1)
	}
}
