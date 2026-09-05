package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/adnoctem/gopskit/pkg/helpers"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	kubehome "k8s.io/client-go/util/homedir"
)

var (
	// OSReleaseRegex is to be used somewhere...
	OSReleaseRegex = `^(?<variable>[A-Za-z\d_-]+)=(?<startQuote>['"]?)(?<value>.[^"']*)(?<endQuote>['"]?)$`

	// KnownConfigTypes are the only file formats we u
	KnownConfigTypes = []string{"yaml", "json"}

	// DefaultConfigTypes is an alias for KnownConfigTypes
	DefaultConfigTypes = KnownConfigTypes
	DefaultAppName     = "gopskit"
	DefaultConfigPaths = []string{
		fmt.Sprintf("/etc/%s", DefaultAppName),
		".",
	}
)

// PlatformPaths represents the platform-specific paths for Config, Data, Cache or Log files or
// directories. If AppName isn't set during initialization we will reuse the generic
// 'gopskit' name to ensure we're writing to a subdirectory of the respective paths.
// This is largely in line with native Platform behaviors.
//
// On Linux we'll use the XDG directories, whereas on Windows and macOS we use system
// variables like %APPDATA% for Windows or $HOME for macOS.
type PlatformPaths struct {
	// AppName the name of the application we're building paths for is this field
	// isn't set during initialization we will set it to DefaultAppName
	AppName string

	// Home is the platform-specific home directory for the current user
	Home string

	// Config is the platform-specific path for configuration files and as such
	// serves as a (one of) source location for waltr configuration files
	Config string

	// Data is the platform-specific path for generic data to be written to and
	// is mainly used a default output directory since it's user-writable
	Data string

	// Cache is the platform-specific path for cache files, which will be read
	// from or written to quite frequently, as such it is also the location for
	// waltr's persistent key-value database
	Cache string

	// Log is the platform-specific path for log files. This path will be used
	// during initialization of the application's logger
	Log string

	// ConfigTypes is a slice of supported file extensions for the app-specific
	// configuration file. If unset during initialization we will set the value
	// to DefaultConfigTypes
	ConfigTypes []string

	// ConfigPaths is a slice of possible source paths for the app-specific
	// configuration file. Each of these paths will be searched for a matching file
	ConfigPaths []string

	// Exists denotes which of the files within ConfigPaths exists by mapping the
	// slices values to map keys and checking for existence of the files
	Exists map[string]bool

	// lock is a Mutex which ensures that only one goroutine at a time may
	// modify the contents of the public fields
	lock sync.Mutex
}

// Opt is a configuration option for the Paths object
type Opt func(p *PlatformPaths)

// DefaultPaths returns a fully initialized PlatformPaths object, using the DefaultAppName as a name
// instead of an actual application name.
//
// Another way of obtaining a Platform object is by using New with the corresponding options.
func DefaultPaths() (*PlatformPaths, error) {
	p := &PlatformPaths{
		AppName:     DefaultAppName,
		ConfigPaths: DefaultConfigPaths,
		ConfigTypes: DefaultConfigTypes,
	}

	if err := p.configure(); err != nil {
		return nil, err
	}

	return p, nil
}

// Paths creates a fully initialized PlatformPaths object with (optional) custom Options to configure
// the final Paths object. If no Options are applied the output will be akin to Default, except
// for the Config... fields which are now set.
func Paths(opts ...Opt) (*PlatformPaths, error) {
	// build and configure the Platform
	p := &PlatformPaths{
		AppName:     DefaultAppName,
		ConfigPaths: DefaultConfigPaths,
		ConfigTypes: DefaultConfigTypes,
		Exists:      make(map[string]bool),
	}

	// keep in line with Windows style decisions
	if runtime.GOOS == "windows" {
		p.AppName = cases.Title(language.Und, cases.Compact).String(p.AppName)
	}

	for _, opt := range opts {
		opt(p)
	}

	p.updateConfigPaths()
	if err := p.configure(); err != nil {
		return nil, err
	}

	if err := p.findConfigFile(); err != nil {
		return nil, err
	}

	return p, nil
}

// WithAppName configures the PlatformPaths object with a custom application name
func WithAppName(app string) Opt {
	return func(p *PlatformPaths) {
		p.lock.Lock()
		defer p.lock.Unlock()
		p.AppName = app
	}
}

// WithConfigPath configures the PlatformPaths object with extra custom configuration paths
// By default we'll use the KnownConfigPaths
func WithConfigPath(path ...string) Opt {
	return func(p *PlatformPaths) {
		p.lock.Lock()
		defer p.lock.Unlock()
		p.ConfigPaths = append(p.ConfigPaths, path...)
	}
}

// WithConfigType configures the PlatformPaths object with a custom configuration type
// By default we assume support JSON and YAML files but this function has the
// ability to lock that to a single value. Only "yaml" or "json" are supported.
func WithConfigType(filetype string) Opt {
	return func(p *PlatformPaths) {
		p.lock.Lock()
		defer p.lock.Unlock()

		// enforce these file types
		if !helpers.SliceContains(DefaultConfigTypes, filetype) {
			fmt.Printf("configuration file type %s not supported\n", filetype)
			os.Exit(1)
		}

		// constrict
		p.ConfigTypes = []string{filetype}
	}
}

// configure prepares the new PlatformPaths object by calling the needed methods
// to fill all public properties
func (p *PlatformPaths) configure() error {
	var err error

	p.lock.Lock()
	defer p.lock.Unlock()

	if p.AppName == "" {
		return errors.New("AppName not provided for Paths object")
	}

	if p.Home, err = p.determineHomeDir(); err != nil {
		return err
	}

	if p.Config, err = p.determineConfigDir(); err != nil {
		return err
	}

	if p.Data, err = p.determineDataDir(); err != nil {
		return err
	}

	if p.Cache, err = p.determineCacheDir(); err != nil {
		return err
	}

	if p.Log, err = p.determineLogDir(); err != nil {
		return err
	}

	return nil
}

// determineHome determines the current user's home directory, respecting the Kubernetes Project's
// sanity checks beforehand. Darwin uses the exact the function
func (p *PlatformPaths) determineHomeDir() (string, error) {
	home := kubehome.HomeDir()

	// use Kubernetes' value if exists
	if home != "" {
		return home, nil
	}

	// still empty? use Go's value then
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	p.Home = home
	return home, nil
}

// determineConfigDir uses the previously discovered home directory to determine a directory for
// configuration files
func (p *PlatformPaths) determineConfigDir() (string, error) {
	conf, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(conf, p.AppName)
	return path, nil
}

// determineCacheDir uses the previously discovered home directory to determine the directory for
// cache files
func (p *PlatformPaths) determineCacheDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(cache, p.AppName)
	return path, nil
}

// updateConfigPaths ensures we use the updated names for the configuration paths,
// whenever an Opt has updated the value post-initialization
func (p *PlatformPaths) updateConfigPaths() {
	p.lock.Lock()
	defer p.lock.Unlock()

	curPaths := p.ConfigPaths
	updated := helpers.RemoveDuplicates(append(curPaths, []string{
		fmt.Sprintf("/etc/%s/%s.%s", p.AppName, p.AppName, p.ConfigTypes[0]),
		fmt.Sprintf("./%s.%s", p.AppName, p.ConfigTypes[0]),
	}...))

	p.ConfigPaths = updated
}

// findConfigFile tries to find a configuration file within the specified ConfigPaths and
// marks the file as existing within the Exists map, if found.
func (p *PlatformPaths) findConfigFile() error {
	p.lock.Lock()
	defer p.lock.Unlock()

	for _, path := range p.ConfigPaths {
		_, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			p.Exists[path] = false
		} else if err != nil {
			p.Exists[path] = false
			return fmt.Errorf("could not stat configuration file: %s. Error: %v", path, err)
		}

		p.Exists[path] = true
	}

	return nil
}
