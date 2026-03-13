package store

import "time"

type Source struct {
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
	NextFetchAt   int64
	BackoffUntil  int64
	LastFetchAt   int64
	ETag          string
	LastModified  string
	EMAYield      float64
	EMAAvgScore   float64
	LastError     string

	FetchOKTotal            int64
	FetchFailTotal          int64
	FetchedTotal            int64
	FetchedNotModifiedTotal int64
}

type Node struct {
	ID          int64
	Kind        string
	Protocol    string
	Fingerprint string
	Host        string
	Port        int
	Username    string
	Password    string
	RawURI      string
	Name        string
	LastSource  int64

	FirstSeenAt   int64
	LastSeenAt    int64
	Status        string
	LastCheckedAt int64
	LastOKAt      int64
	LatencyMS     int
	ExitIP        string
	Country       string
	ASN           string
	Anonymity     string
	PurityScore   int
	Score         float64
	OKCount       int64
	FailCount     int64
	FailStreak    int
	BanUntil      int64
	LastError     string
}

type Check struct {
	NodeID      int64
	CheckedAt   int64
	OK          bool
	LatencyMS   int
	ExitIP      string
	Country     string
	Anonymity   string
	PurityScore int
	Error       string
}

type IPFacts struct {
	IP        string
	UpdatedAt int64
	Country   string
	Proxy     bool
	Hosting   bool
	Mobile    bool
}

func (n Node) IsBanned(now time.Time) bool {
	return n.BanUntil > 0 && n.BanUntil > now.Unix()
}

