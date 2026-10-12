package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/adnoctem/gopskit/internal/carto/app"
	"github.com/adnoctem/gopskit/internal/carto/cmd"
	_ "github.com/adnoctem/gopskit/pkg/stamp"
)

func main() {
	kern, err := app.New()
	if err != nil {
		log.Fatalf("couldn't initialize %s!\nError: %v", app.Name, err)
	}

	cmdRoot := cmd.NewRootCommand(kern)
	if err := cmdRoot.Execute(); err != nil {
		kern.Log.Fatalf("command: '%s' resulted in error: %v\n", commandName(), err)
	}

	os.Exit(0)
}

// commandName is the subcommand the user ran, for error messages - the CLI's own name when none
// was given.
func commandName() string {
	if len(os.Args) > 1 {
		return os.Args[1]
	}

	return filepath.Base(os.Args[0])
}
