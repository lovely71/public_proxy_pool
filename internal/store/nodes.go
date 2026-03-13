package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type NodeUpsert struct {
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
	Country     string
	LatencyMS   int
}

func (s *Store) UpsertNodes(ctx context.Context, now time.Time, nodes []NodeUpsert) (int, error) {
	if len(nodes) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO nodes(kind,protocol,fingerprint,host,port,username,password,raw_uri,name,last_source_id,first_seen_at,last_seen_at,status,country,latency_ms)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,'unknown',?,?)
ON CONFLICT(fingerprint) DO UPDATE SET
  last_seen_at=excluded.last_seen_at,
  last_source_id=excluded.last_source_id,
  raw_uri=excluded.raw_uri,
  name=CASE WHEN excluded.name<>'' THEN excluded.name ELSE nodes.name END,
  country=CASE WHEN (nodes.country='' OR nodes.country IS NULL) AND excluded.country<>'' THEN excluded.country ELSE nodes.country END,
  latency_ms=CASE WHEN (nodes.latency_ms=0 OR nodes.latency_ms IS NULL) AND excluded.latency_ms>0 THEN excluded.latency_ms ELSE nodes.latency_ms END
`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	seen := make(map[string]struct{}, len(nodes))
	nInserted := 0
	for _, n := range nodes {
		fp := strings.TrimSpace(n.Fingerprint)
		if fp == "" {
			continue
		}
		if _, ok := seen[fp]; ok {
			continue
		}
		seen[fp] = struct{}{}
		if _, err := stmt.ExecContext(ctx,
			n.Kind, n.Protocol, fp, n.Host, n.Port, n.Username, n.Password, n.RawURI, n.Name, n.LastSource,
			now.Unix(), now.Unix(),
			strings.ToUpper(strings.TrimSpace(n.Country)),
			n.LatencyMS,
		); err != nil {
			return 0, err
		}
		nInserted++
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return nInserted, nil
}

type NodeFilter struct {
	Protocols   []string
	Countries   []string
	MinPurity   int
	MaxLatency  int
	Kind        string
	FreshWithin time.Duration
	Verify      bool
}

func (f NodeFilter) normalize() NodeFilter {
	out := f
	if out.MinPurity < 0 {
		out.MinPurity = 0
	}
	if out.MinPurity > 100 {
		out.MinPurity = 100
	}
	return out
}

func (s *Store) QueryFreshValidNodes(ctx context.Context, now time.Time, filter NodeFilter, limit int) ([]Node, error) {
	filter = filter.normalize()
	if limit <= 0 {
		limit = 20
	}

	var args []any
	where := []string{
		"ban_until<=?",
	}
	args = append(args, now.Unix())

	if filter.Kind != "" {
		where = append(where, "kind=?")
		args = append(args, filter.Kind)
	}
	if len(filter.Protocols) > 0 {
		where = append(where, "protocol IN ("+placeholders(len(filter.Protocols))+")")
		for _, p := range filter.Protocols {
			args = append(args, p)
		}
	}
	if len(filter.Countries) > 0 {
		where = append(where, "country IN ("+placeholders(len(filter.Countries))+")")
		for _, c := range filter.Countries {
			args = append(args, c)
		}
	}
	if filter.MinPurity > 0 {
		where = append(where, "purity_score>=?")
		args = append(args, filter.MinPurity)
	}
	if filter.MaxLatency > 0 {
		where = append(where, "latency_ms>0 AND latency_ms<=?")
		args = append(args, filter.MaxLatency)
	}

	if filter.Verify {
		where = append(where, "status='valid'")
		freshSeconds := int64(filter.FreshWithin.Seconds())
		if freshSeconds <= 0 {
			freshSeconds = int64((5 * time.Minute).Seconds())
		}
		where = append(where, "last_checked_at>=?")
		args = append(args, now.Unix()-freshSeconds)
	}

	query := `
SELECT id,kind,protocol,fingerprint,host,port,username,password,raw_uri,name,last_source_id,first_seen_at,last_seen_at,status,last_checked_at,last_ok_at,latency_ms,exit_ip,country,asn,anonymity,purity_score,score,ok_count,fail_count,fail_streak,ban_until,last_error
FROM nodes
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY score DESC, latency_ms ASC, last_ok_at DESC
LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(
			&n.ID, &n.Kind, &n.Protocol, &n.Fingerprint, &n.Host, &n.Port, &n.Username, &n.Password, &n.RawURI, &n.Name, &n.LastSource,
			&n.FirstSeenAt, &n.LastSeenAt, &n.Status, &n.LastCheckedAt, &n.LastOKAt, &n.LatencyMS, &n.ExitIP, &n.Country, &n.ASN,
			&n.Anonymity, &n.PurityScore, &n.Score, &n.OKCount, &n.FailCount, &n.FailStreak, &n.BanUntil, &n.LastError,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) QueryValidationCandidates(ctx context.Context, now time.Time, filter NodeFilter, limit int) ([]Node, error) {
	filter = filter.normalize()
	if limit <= 0 {
		limit = 100
	}
	var args []any
	where := []string{
		"ban_until<=?",
	}
	args = append(args, now.Unix())

	if filter.Kind != "" {
		where = append(where, "kind=?")
		args = append(args, filter.Kind)
	}
	if len(filter.Protocols) > 0 {
		where = append(where, "protocol IN ("+placeholders(len(filter.Protocols))+")")
		for _, p := range filter.Protocols {
			args = append(args, p)
		}
	}
	if len(filter.Countries) > 0 {
		where = append(where, "country IN ("+placeholders(len(filter.Countries))+")")
		for _, c := range filter.Countries {
			args = append(args, c)
		}
	}
	if filter.MinPurity > 0 {
		where = append(where, "purity_score>=?")
		args = append(args, filter.MinPurity)
	}
	if filter.MaxLatency > 0 {
		where = append(where, "(latency_ms=0 OR latency_ms<=?)")
		args = append(args, filter.MaxLatency)
	}

	// Candidates: unknown, invalid (maybe recovered), or stale checks.
	freshSeconds := int64(filter.FreshWithin.Seconds())
	if freshSeconds <= 0 {
		freshSeconds = int64((5 * time.Minute).Seconds())
	}
	where = append(where, "(status<>'valid' OR last_checked_at<?)")
	args = append(args, now.Unix()-freshSeconds)

	query := `
SELECT id,kind,protocol,fingerprint,host,port,username,password,raw_uri,name,last_source_id,first_seen_at,last_seen_at,status,last_checked_at,last_ok_at,latency_ms,exit_ip,country,asn,anonymity,purity_score,score,ok_count,fail_count,fail_streak,ban_until,last_error
FROM nodes
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY score DESC, last_checked_at ASC, fail_streak ASC
LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(
			&n.ID, &n.Kind, &n.Protocol, &n.Fingerprint, &n.Host, &n.Port, &n.Username, &n.Password, &n.RawURI, &n.Name, &n.LastSource,
			&n.FirstSeenAt, &n.LastSeenAt, &n.Status, &n.LastCheckedAt, &n.LastOKAt, &n.LatencyMS, &n.ExitIP, &n.Country, &n.ASN,
			&n.Anonymity, &n.PurityScore, &n.Score, &n.OKCount, &n.FailCount, &n.FailStreak, &n.BanUntil, &n.LastError,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) GetNodeByID(ctx context.Context, id int64) (*Node, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id,kind,protocol,fingerprint,host,port,username,password,raw_uri,name,last_source_id,first_seen_at,last_seen_at,status,last_checked_at,last_ok_at,latency_ms,exit_ip,country,asn,anonymity,purity_score,score,ok_count,fail_count,fail_streak,ban_until,last_error
FROM nodes WHERE id=?`, id)
	var n Node
	if err := row.Scan(
		&n.ID, &n.Kind, &n.Protocol, &n.Fingerprint, &n.Host, &n.Port, &n.Username, &n.Password, &n.RawURI, &n.Name, &n.LastSource,
		&n.FirstSeenAt, &n.LastSeenAt, &n.Status, &n.LastCheckedAt, &n.LastOKAt, &n.LatencyMS, &n.ExitIP, &n.Country, &n.ASN,
		&n.Anonymity, &n.PurityScore, &n.Score, &n.OKCount, &n.FailCount, &n.FailStreak, &n.BanUntil, &n.LastError,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &n, nil
}

func (s *Store) GetNodeByFingerprint(ctx context.Context, fp string) (*Node, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id,kind,protocol,fingerprint,host,port,username,password,raw_uri,name,last_source_id,first_seen_at,last_seen_at,status,last_checked_at,last_ok_at,latency_ms,exit_ip,country,asn,anonymity,purity_score,score,ok_count,fail_count,fail_streak,ban_until,last_error
FROM nodes WHERE fingerprint=?`, fp)
	var n Node
	if err := row.Scan(
		&n.ID, &n.Kind, &n.Protocol, &n.Fingerprint, &n.Host, &n.Port, &n.Username, &n.Password, &n.RawURI, &n.Name, &n.LastSource,
		&n.FirstSeenAt, &n.LastSeenAt, &n.Status, &n.LastCheckedAt, &n.LastOKAt, &n.LatencyMS, &n.ExitIP, &n.Country, &n.ASN,
		&n.Anonymity, &n.PurityScore, &n.Score, &n.OKCount, &n.FailCount, &n.FailStreak, &n.BanUntil, &n.LastError,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &n, nil
}

type NodeCheckUpdate struct {
	CheckedAt   int64
	OK          bool
	LatencyMS   int
	ExitIP      string
	Country     string
	ASN         string
	Anonymity   string
	PurityScore int
	Error       string
	Score       float64
}

func (s *Store) ApplyNodeCheck(ctx context.Context, nodeID int64, up NodeCheckUpdate) error {
	okInc := int64(0)
	failInc := int64(0)
	status := "invalid"
	lastOKAt := int64(0)
	if up.OK {
		okInc = 1
		status = "valid"
		lastOKAt = up.CheckedAt
	} else {
		failInc = 1
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO checks(node_id,checked_at,ok,latency_ms,exit_ip,country,anonymity,purity_score,error)
VALUES(?,?,?,?,?,?,?,?,?)`,
		nodeID,
		up.CheckedAt,
		boolToInt(up.OK),
		up.LatencyMS,
		up.ExitIP,
		strings.ToUpper(strings.TrimSpace(up.Country)),
		up.Anonymity,
		up.PurityScore,
		up.Error,
	); err != nil {
		return err
	}

	if up.PurityScore < 0 {
		up.PurityScore = 0
	}
	if up.PurityScore > 100 {
		up.PurityScore = 100
	}

	var query string
	var args []any
	if up.OK {
		query = `
UPDATE nodes SET
  status=?,
  last_checked_at=?,
  last_ok_at=?,
  latency_ms=?,
  exit_ip=?,
  country=?,
  asn=?,
  anonymity=?,
  purity_score=?,
  score=?,
  ok_count=ok_count+?,
  fail_count=fail_count+?,
  fail_streak=0,
  last_error=''
WHERE id=?`
		args = []any{status, up.CheckedAt, lastOKAt, up.LatencyMS, up.ExitIP, strings.ToUpper(strings.TrimSpace(up.Country)), strings.TrimSpace(up.ASN), up.Anonymity, up.PurityScore, up.Score, okInc, failInc, nodeID}
	} else {
		query = `
UPDATE nodes SET
  status=?,
  last_checked_at=?,
  latency_ms=?,
  exit_ip=?,
  country=?,
  asn=?,
  anonymity=?,
  purity_score=?,
  score=?,
  ok_count=ok_count+?,
  fail_count=fail_count+?,
  fail_streak=CASE WHEN fail_streak<0 THEN 1 ELSE fail_streak+1 END,
  last_error=?
WHERE id=?`
		args = []any{status, up.CheckedAt, up.LatencyMS, up.ExitIP, strings.ToUpper(strings.TrimSpace(up.Country)), strings.TrimSpace(up.ASN), up.Anonymity, up.PurityScore, up.Score, okInc, failInc, up.Error, nodeID}
	}

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (s *Store) BanNode(ctx context.Context, nodeID int64, until time.Time, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE nodes SET ban_until=?, last_error=? WHERE id=?`, until.Unix(), reason, nodeID)
	return err
}

func (s *Store) UnbanNode(ctx context.Context, nodeID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE nodes SET ban_until=0 WHERE id=?`, nodeID)
	return err
}

func placeholders(n int) string {
	if n <= 0 {
		return "?"
	}
	sb := strings.Builder{}
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("?")
	}
	return sb.String()
}
