package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captured holds the flag values a test command saw when it ran.
type captured struct {
	url      string
	realm    string
	outputs  []string
	public   bool
	timeout  time.Duration
	required string
}

// newTree builds `app [--config] [--url] client apply [--realm --output --public --timeout --uri]`
// with the configuration loaded the way a CLI's root command loads it.
func newTree(t *testing.T) (*cobra.Command, *captured) {
	t.Helper()

	got := &captured{}

	root := &cobra.Command{
		Use:           "app",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return Load("app", cmd)
		},
	}
	AddFlag(root, "app")
	root.PersistentFlags().StringVar(&got.url, "url", "default-url", "")
	BindEnv(root.PersistentFlags(), "url", "APP_URL")

	client := &cobra.Command{Use: "client"}
	apply := &cobra.Command{
		Use:     "apply",
		Aliases: []string{"create"},
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	apply.Flags().StringVar(&got.realm, "realm", "default-realm", "")
	apply.Flags().StringSliceVar(&got.outputs, "output", []string{"stdout"}, "")
	apply.Flags().BoolVar(&got.public, "public", false, "")
	apply.Flags().DurationVar(&got.timeout, "timeout", time.Second, "")
	apply.Flags().StringVar(&got.required, "uri", "", "")
	require.NoError(t, apply.MarkFlagRequired("uri"))

	client.AddCommand(apply)
	root.AddCommand(client)

	return root, got
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "app.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func execute(t *testing.T, root *cobra.Command, args ...string) error {
	t.Helper()

	// keep the search away from any real configuration in the working directory
	t.Chdir(t.TempDir())

	root.SetArgs(args)
	return root.Execute()
}

func TestSettingsReachFlags(t *testing.T) {
	asrt := assert.New(t)
	root, got := newTree(t)

	path := writeConfig(t, `
url: https://from-file
client:
  apply:
    realm: applications
    output: [kubernetes, vault]
    public: true
    timeout: 2m
    uri: https://app/cb
`)

	require.NoError(t, execute(t, root, "client", "apply", "--config", path))
	asrt.Equal("https://from-file", got.url)
	asrt.Equal("applications", got.realm)
	asrt.Equal([]string{"kubernetes", "vault"}, got.outputs)
	asrt.True(got.public)
	asrt.Equal(2*time.Minute, got.timeout)
	asrt.Equal("https://app/cb", got.required, "a setting must satisfy a required flag")
}

func TestPrecedence(t *testing.T) {
	path := writeConfig(t, `
url: https://from-file
client:
  apply:
    uri: https://app/cb
`)

	t.Run("the command line beats everything", func(t *testing.T) {
		root, got := newTree(t)
		t.Setenv("APP_URL", "https://from-env")

		require.NoError(t, execute(t, root, "client", "apply", "--config", path, "--url", "https://from-flag"))
		assert.Equal(t, "https://from-flag", got.url)
	})

	t.Run("the environment beats the file", func(t *testing.T) {
		root, got := newTree(t)
		t.Setenv("APP_URL", "https://from-env")

		require.NoError(t, execute(t, root, "client", "apply", "--config", path))
		assert.Equal(t, "https://from-env", got.url)
	})

	t.Run("the file beats the default", func(t *testing.T) {
		root, got := newTree(t)

		require.NoError(t, execute(t, root, "client", "apply", "--config", path))
		assert.Equal(t, "https://from-file", got.url)
	})

	t.Run("without any, the default stands", func(t *testing.T) {
		root, got := newTree(t)

		require.NoError(t, execute(t, root, "client", "apply", "--uri", "https://app/cb"))
		assert.Equal(t, "default-url", got.url)
		assert.Equal(t, []string{"stdout"}, got.outputs)
	})
}

func TestMoreSpecificSettingsWin(t *testing.T) {
	root, got := newTree(t)

	path := writeConfig(t, `
realm: everywhere
client:
  realm: client-commands
  apply:
    realm: apply-only
    uri: https://app/cb
`)

	require.NoError(t, execute(t, root, "client", "apply", "--config", path))
	assert.Equal(t, "apply-only", got.realm)
}

func TestSectionsMayUseAliases(t *testing.T) {
	root, got := newTree(t)

	path := writeConfig(t, `
client:
  create:
    realm: via-alias
    uri: https://app/cb
`)

	require.NoError(t, execute(t, root, "client", "apply", "--config", path))
	assert.Equal(t, "via-alias", got.realm)
}

func TestValidationCatchesTypos(t *testing.T) {
	cases := map[string]string{
		"unknown flag":       "urll: x\n",
		"unknown subcommand": "server:\n  apply:\n    realm: x\n",
		"flag in wrong spot": "client:\n  apply:\n    url2: x\n",
		"nested config":      "config: other.yaml\n",
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			root, _ := newTree(t)
			err := execute(t, root, "client", "apply", "--config", writeConfig(t, content), "--uri", "x")
			assert.Error(t, err)
		})
	}
}

func TestBadValuesAreReported(t *testing.T) {
	root, _ := newTree(t)

	path := writeConfig(t, "client:\n  apply:\n    timeout: soon\n    uri: x\n")
	err := execute(t, root, "client", "apply", "--config", path)
	assert.ErrorContains(t, err, "timeout")
}

func TestExplicitFileMustExist(t *testing.T) {
	root, _ := newTree(t)

	err := execute(t, root, "client", "apply", "--config", "/does/not/exist.yaml", "--uri", "x")
	assert.ErrorContains(t, err, "configuration file")
}

func TestFindsFileInWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile("app.yaml", []byte("url: https://cwd\n"), 0o600))

	path, err := Find("app", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(".", "app.yaml"), path)
}

func TestNoFileIsFine(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	path, err := Find("app-that-has-no-config", "")
	require.NoError(t, err)
	assert.Empty(t, path)
}

type testDocument struct {
	Policies map[string]string `yaml:"policies"`
	Roles    []struct {
		Name string `yaml:"name"`
	} `yaml:"roles"`
}

// newDocumentTree is newTree with a "state" document registered and loaded by the root.
func newDocumentTree(t *testing.T) (*cobra.Command, *captured) {
	t.Helper()

	root, got := newTree(t)
	AddDocument(root, "state")

	return root, got
}

func TestDocumentSections(t *testing.T) {
	asrt := assert.New(t)

	path := writeConfig(t, `
url: https://from-file
state:
  policies:
    admin: path "*" {}
  roles:
    - name: app
client:
  apply:
    uri: https://app/cb
`)

	root, got := newDocumentTree(t)
	require.NoError(t, execute(t, root, "client", "apply", "--config", path))
	asrt.Equal("https://from-file", got.url, "the flag settings next to a document still apply")

	var doc testDocument
	present, err := Section(root, "state", &doc)
	require.NoError(t, err)
	asrt.True(present)
	asrt.Equal(`path "*" {}`, doc.Policies["admin"])
	require.Len(t, doc.Roles, 1)
	asrt.Equal("app", doc.Roles[0].Name)
}

func TestDocumentsAreStrict(t *testing.T) {
	path := writeConfig(t, `
state:
  policies:
    admin: x
  rolls:
    - name: app
`)

	root, _ := newDocumentTree(t)
	require.NoError(t, execute(t, root, "client", "apply", "--config", path, "--uri", "x"))

	var doc testDocument
	_, err := Section(root, "state", &doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rolls", "a misspelled key must be an error")
	assert.Contains(t, err.Error(), "line 5", "with the line number of the file itself")
}

func TestMissingDocument(t *testing.T) {
	root, _ := newDocumentTree(t)
	require.NoError(t, execute(t, root, "client", "apply", "--config", writeConfig(t, "url: x\n"), "--uri", "x"))

	var doc testDocument
	present, err := Section(root, "state", &doc)
	require.NoError(t, err)
	assert.False(t, present)
}

func TestUnregisteredDocumentIsAnError(t *testing.T) {
	root, _ := newTree(t)

	err := execute(t, root, "client", "apply", "--config", writeConfig(t, "state:\n  policies: {}\n"), "--uri", "x")
	assert.Error(t, err, "without AddDocument, a mapping must name a subcommand")
}
