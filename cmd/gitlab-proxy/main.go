package main

import (
	"os"

	assets "gitlab-proxy"
	"gitlab-proxy/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, assets.Skills()))
}
