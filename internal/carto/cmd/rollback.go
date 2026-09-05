package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/adnoctem/gopskit/internal/carto/app"
	"github.com/adnoctem/gopskit/internal/carto/config"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewRollbackCommand // assure type compatibility

// NewRollbackCommand creates the Option which injects the 'rollback' subcommand into the 'carto' CLI
func NewRollbackCommand(a *app.State) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:              "rollback <chart> <revision>",
		Short:            "Roll a chart's release back to a previous revision",
		Args:             cobra.ExactArgs(2),
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			revision, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid revision %q: %w", args[1], err)
			}

			configPath := proc.Must(cmd.Flags().GetString("config"))

			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			cc, err := chartConfig(cfg, name)
			if err != nil {
				return err
			}

			if !yes {
				confirmed, err := confirm(fmt.Sprintf("roll back release %q to revision %d?", name, revision))
				if err != nil {
					return err
				}
				if !confirmed {
					a.Log.Info("rollback aborted")
					return nil
				}
			}

			hc, err := a.HelmClient(cc.Namespace)
			if err != nil {
				return err
			}

			if err := hc.Rollback(name, revision); err != nil {
				return fmt.Errorf("could not roll back chart %q to revision %d: %w", name, revision, err)
			}

			a.Log.Infof("successfully rolled back chart %q to revision %d", name, revision)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

// confirm asks the user a yes/no question on stdin, defaulting to "no" on any non-"y" answer
func confirm(prompt string) (bool, error) {
	fmt.Printf("%s [y/N]: ", prompt)

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}
