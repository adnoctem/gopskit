package stamp

var (
	Version   = "redacted"
	BuildDate = "redacted"
	CommitSHA = "redacted"
	Branch    = "redacted"
	Platform  = "redacted"
	GoVersion = "redacted"
)

type Stamps struct {
	Version   string
	BuildDate string
	CommitSHA string
	Branch    string
	Platform  string
	GoVersion string
}

// New returns a new Stamps object which can be embedded in other structs
func New() *Stamps {
	return &Stamps{
		Version:   Version,
		BuildDate: BuildDate,
		CommitSHA: CommitSHA,
		Branch:    Branch,
		Platform:  Platform,
		GoVersion: GoVersion,
	}
}
