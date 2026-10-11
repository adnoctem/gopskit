// Package config gives the gopskit CLIs a configuration file mirroring their flags.
//
// The file is YAML. Its keys are flag names; a key whose value is a mapping and that names a
// subcommand scopes the settings within it to that subcommand. Top-level settings apply to every
// command that has the flag:
//
//	url: https://keycloak.keycloak.svc:8443   # every command with --url
//	via: bridge
//
//	client:
//	  apply:                                  # only `ssolo client apply`
//	    realm: applications
//	    output: [kubernetes, vault]
//
// Because the keys are the flags themselves, every flag - including ones added later - is
// configurable without wiring of its own, and a setting means exactly what the flag means.
//
// A value is taken from, in order: the command line, the flag's environment variable (see
// BindEnv), the most specific setting in the file, and finally the flag's default.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// FlagName is the flag that names an explicit configuration file.
const FlagName = "config"

// envAnnotation marks a flag with the environment variable that may set it.
const envAnnotation = "gopskit.adnoctem.io/env"

// Settings is a parsed configuration file.
type Settings map[string]any

// AddFlag registers --config on cmd, normally the root command, as a persistent flag.
func AddFlag(cmd *cobra.Command, app string) {
	cmd.PersistentFlags().String(FlagName, "", fmt.Sprintf(
		"The configuration file (default: the first of %s)", strings.Join(SearchPaths(app), ", ")))
}

// BindEnv lets the environment variable env set the flag name, unless it is given on the command
// line. The flag must already be defined in fs.
func BindEnv(fs *pflag.FlagSet, name, env string) {
	f := fs.Lookup(name)
	if f == nil {
		panic(fmt.Sprintf("config.BindEnv: no flag %q", name))
	}

	if f.Annotations == nil {
		f.Annotations = map[string][]string{}
	}
	f.Annotations[envAnnotation] = []string{env}
	f.Usage = fmt.Sprintf("%s (env %s)", f.Usage, env)
}

// SearchPaths are the places a configuration file for app is looked for, most specific first:
// the working directory, the user's configuration directory, and /etc.
func SearchPaths(app string) []string {
	file := app + ".yaml"
	paths := []string{filepath.Join(".", file)}

	if dir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(dir, app, file))
	}

	return append(paths, filepath.Join("/etc", app, file))
}

// Find returns the configuration file to use: explicit if set, which must then exist, or else the
// first of SearchPaths that exists. It returns "" when there is none.
func Find(app, explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("configuration file: %w", err)
		}
		return explicit, nil
	}

	for _, p := range SearchPaths(app) {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("configuration file %s: %w", p, err)
		}
	}

	return "", nil
}

// Read parses the configuration file at path.
func Read(path string) (Settings, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// decode into a plain map: yaml.v3 gives nested mappings the type of their parent, and
	// sections are recognised as map[string]any
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("configuration file %s: %w", path, err)
	}

	return Settings(m), nil
}

// Load is what a CLI's root command calls before a command runs: it finds and reads the
// configuration file, checks it against the command tree and applies it to cmd's flags.
func Load(app string, cmd *cobra.Command) error {
	explicit, _ := cmd.Flags().GetString(FlagName)

	path, err := Find(app, explicit)
	if err != nil {
		return err
	}

	var s Settings
	if path != "" {
		if s, err = Read(path); err != nil {
			return err
		}
		if err := Validate(cmd.Root(), s); err != nil {
			return fmt.Errorf("configuration file %s: %w", path, err)
		}
	}

	if err := Apply(cmd, s); err != nil {
		if path != "" {
			return fmt.Errorf("configuration file %s: %w", path, err)
		}
		return err
	}

	return nil
}

// Apply sets every flag of cmd not given on the command line from its environment variable or,
// failing that, from the most specific setting in s for cmd. s may be nil.
func Apply(cmd *cobra.Command, s Settings) error {
	scopes := scopesFor(cmd, s)

	var errs []error
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Changed || f.Name == FlagName {
			return
		}

		if env := f.Annotations[envAnnotation]; len(env) > 0 {
			if v, ok := os.LookupEnv(env[0]); ok && v != "" {
				if err := cmd.Flags().Set(f.Name, v); err != nil {
					errs = append(errs, fmt.Errorf("$%s: %w", env[0], err))
				}
				return
			}
		}

		for _, scope := range scopes {
			v, ok := scope[f.Name]
			if !ok {
				continue
			}
			if _, isSection := v.(map[string]any); isSection {
				continue
			}

			if err := set(cmd.Flags(), f, v); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", f.Name, err))
			}
			return
		}
	})

	return errors.Join(errs...)
}

// scopesFor returns the sections of s that apply to cmd, most specific first: for
// `ssolo client apply` that is s["client"]["apply"], s["client"] and s itself.
func scopesFor(cmd *cobra.Command, s Settings) []map[string]any {
	if s == nil {
		return nil
	}

	var path []*cobra.Command
	for c := cmd; c.HasParent(); c = c.Parent() {
		path = append([]*cobra.Command{c}, path...)
	}

	scopes := []map[string]any{s}
	current := map[string]any(s)
	for _, c := range path {
		next := section(current, c)
		if next == nil {
			break
		}
		scopes = append(scopes, next)
		current = next
	}

	slices.Reverse(scopes)
	return scopes
}

// section returns the settings for c within current, written under its name or an alias.
func section(current map[string]any, c *cobra.Command) map[string]any {
	for _, name := range append([]string{c.Name()}, c.Aliases...) {
		if next, ok := current[name].(map[string]any); ok {
			return next
		}
	}

	return nil
}

func set(fs *pflag.FlagSet, f *pflag.Flag, v any) error {
	if list, ok := v.([]any); ok {
		items := make([]string, len(list))
		for i, item := range list {
			if _, nested := item.(map[string]any); nested {
				return errors.New("list items must be plain values")
			}
			items[i] = fmt.Sprint(item)
		}

		sv, ok := f.Value.(pflag.SliceValue)
		if !ok {
			return fmt.Errorf("takes a single value, not a list")
		}
		if err := sv.Replace(items); err != nil {
			return err
		}
		f.Changed = true
		return nil
	}

	if v == nil {
		return errors.New("has no value")
	}

	return fs.Set(f.Name, fmt.Sprint(v))
}

// Validate checks that every key in s is a subcommand or a flag that exists where it is
// written, so a typo in the file is an error instead of a setting silently ignored.
func Validate(root *cobra.Command, s Settings) error {
	return validate(root, s, nil)
}

func validate(cmd *cobra.Command, section map[string]any, at []string) error {
	keys := make([]string, 0, len(section))
	for k := range section {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var errs []error
	for _, key := range keys {
		where := strings.Join(append(append([]string{}, at...), key), ".")

		if sub, ok := section[key].(map[string]any); ok {
			child := subcommand(cmd, key)
			if child == nil {
				errs = append(errs, fmt.Errorf("%s: %q is not a subcommand of %q", where, key, cmd.CommandPath()))
				continue
			}
			if err := validate(child, sub, append(at, key)); err != nil {
				errs = append(errs, err)
			}
			continue
		}

		if key == FlagName {
			errs = append(errs, fmt.Errorf("%s: the configuration file can't name another one", where))
			continue
		}
		if !hasFlag(cmd, key) {
			errs = append(errs, fmt.Errorf("%s: no command under %q has a --%s flag", where, cmd.CommandPath(), key))
		}
	}

	return errors.Join(errs...)
}

func subcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, c := range cmd.Commands() {
		if c.Name() == name || slices.Contains(c.Aliases, name) {
			return c
		}
	}

	return nil
}

// hasFlag reports whether cmd or any command below it accepts --name, including flags it
// inherits, which is where a setting written at cmd's level can take effect.
func hasFlag(cmd *cobra.Command, name string) bool {
	if cmd.Flags().Lookup(name) != nil || cmd.InheritedFlags().Lookup(name) != nil ||
		cmd.PersistentFlags().Lookup(name) != nil {
		return true
	}

	for _, c := range cmd.Commands() {
		if hasFlag(c, name) {
			return true
		}
	}

	return false
}
