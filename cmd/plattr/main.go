package main

import (
	"log"
	"os"
	"strings"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/internal/plattr/cmd"
	_ "github.com/adnoctem/gopskit/pkg/stamp"
)

func main() {
	kern, err := app.New()
	if err != nil {
		log.Fatalf("ERROR: couldn't initialize %s!\nError: %v", app.Name, err)
	}

	cmdRoot := cmd.NewRootCommand(kern)
	if err := cmdRoot.Execute(); err != nil {
		kern.Log.Fatalf("command: '%s' resulted in error: %v\n", strings.Join(os.Args[1:2], " "), err)
	}

	os.Exit(0)
}
