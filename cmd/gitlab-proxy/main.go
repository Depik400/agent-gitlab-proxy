package main

import (
	"os"

	assets "github.com/Depik400/agent-gitlab-proxy"
	"github.com/Depik400/agent-gitlab-proxy/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, assets.Skills()))
}
