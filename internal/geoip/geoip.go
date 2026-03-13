package geoip

import (
	"fmt"
	"net"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

type DB struct {
	country *geoip2.Reader
	asn     *geoip2.Reader
}

func Open(countryMMDBPath, asnMMDBPath string) (*DB, error) {
	countryMMDBPath = strings.TrimSpace(countryMMDBPath)
	asnMMDBPath = strings.TrimSpace(asnMMDBPath)
	if countryMMDBPath == "" && asnMMDBPath == "" {
		return nil, nil
	}

	var out DB
	if countryMMDBPath != "" {
		r, err := geoip2.Open(countryMMDBPath)
		if err != nil {
			return nil, fmt.Errorf("open geoip country mmdb: %w", err)
		}
		out.country = r
	}
	if asnMMDBPath != "" {
		r, err := geoip2.Open(asnMMDBPath)
		if err != nil {
			if out.country != nil {
				_ = out.country.Close()
			}
			return nil, fmt.Errorf("open geoip asn mmdb: %w", err)
		}
		out.asn = r
	}
	return &out, nil
}

func (d *DB) Close() error {
	if d == nil {
		return nil
	}
	var firstErr error
	if d.country != nil {
		if err := d.country.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if d.asn != nil {
		if err := d.asn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (d *DB) CountryISO(ipStr string) string {
	if d == nil || d.country == nil {
		return ""
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return ""
	}
	rec, err := d.country.Country(ip)
	if err != nil {
		return ""
	}
	code := strings.TrimSpace(rec.Country.IsoCode)
	if code == "" {
		code = strings.TrimSpace(rec.RegisteredCountry.IsoCode)
	}
	return strings.ToUpper(code)
}

func (d *DB) ASN(ipStr string) string {
	if d == nil || d.asn == nil {
		return ""
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return ""
	}
	rec, err := d.asn.ASN(ip)
	if err != nil {
		return ""
	}
	if rec.AutonomousSystemNumber == 0 {
		return ""
	}
	return fmt.Sprintf("AS%d", rec.AutonomousSystemNumber)
}

