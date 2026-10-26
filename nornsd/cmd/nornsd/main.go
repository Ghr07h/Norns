package main

import (
	"os"

	"github.com/Ghr07h/norns/nornsd/internal/cli"
)

func main() { os.Exit(cli.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
