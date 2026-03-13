package store

import (
	"context"
	"database/sql"
	"time"
)

func (s *Store) GetIPFacts(ctx context.Context, ip string) (*IPFacts, error) {
	row := s.db.QueryRowContext(ctx, `SELECT ip,updated_at,country,proxy,hosting,mobile FROM ip_facts WHERE ip=?`, ip)
	var out IPFacts
	var proxyI, hostingI, mobileI int64
	if err := row.Scan(&out.IP, &out.UpdatedAt, &out.Country, &proxyI, &hostingI, &mobileI); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	out.Proxy = intToBool(proxyI)
	out.Hosting = intToBool(hostingI)
	out.Mobile = intToBool(mobileI)
	return &out, nil
}

func (s *Store) UpsertIPFacts(ctx context.Context, facts IPFacts) error {
	if facts.UpdatedAt <= 0 {
		facts.UpdatedAt = time.Now().Unix()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO ip_facts(ip,updated_at,country,proxy,hosting,mobile)
VALUES(?,?,?,?,?,?)
ON CONFLICT(ip) DO UPDATE SET
  updated_at=excluded.updated_at,
  country=excluded.country,
  proxy=excluded.proxy,
  hosting=excluded.hosting,
  mobile=excluded.mobile
`, facts.IP, facts.UpdatedAt, facts.Country, boolToInt(facts.Proxy), boolToInt(facts.Hosting), boolToInt(facts.Mobile))
	return err
}

