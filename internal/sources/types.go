package sources

// SourceDef is the config-level representation used by the fetcher layer.
type SourceDef struct {
	ID            int64
	Name          string
	Type          string
	URL           string
	Parser        string
	DefaultScheme string
	RepoURL       string
	UpdateHint    string
	Enabled       bool
	IntervalSec   int
	ETag          string
	LastModified  string
}

