package main

import (
	"log"

	"github.com/adnoctem/gopskit/internal/fillr/app"
	"github.com/adnoctem/gopskit/internal/fillr/cmd"
	_ "github.com/adnoctem/gopskit/pkg/stamp"
)

func main() {
	kern, err := app.New()
	if err != nil {
		log.Fatalf("could not initialize %s: %v", app.Name, err)
	}

	cmdRoot := cmd.NewRootCommand(kern)
	if err := cmdRoot.Execute(); err != nil {
		kern.Log.Fatalf("%s exited with error: %v\n", kern.Name, err)
	}
}
