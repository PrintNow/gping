package geoip

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"

	"gping/data"

	"github.com/oschwald/maxminddb-golang/v2"
)

type CityInfo struct {
	Country  string
	Province string
	City     string
}

type GeoIPLookup struct {
	cityReader *maxminddb.Reader
}

func NewGeoIPLookup() (*GeoIPLookup, error) {
	cityReader, err := openCityReader()
	if err != nil {
		return nil, err
	}

	return &GeoIPLookup{
		cityReader: cityReader,
	}, nil
}

// openCityReader loads GeoLite2 City in order: GEOIP_CITY_DB, data/GeoLite2-City.mmdb
// (cwd and beside executable), then compile-time embedded copy. Embedding can go stale when
// only the MMDB changes if the build cache is wrong; filesystem paths pick up replacements
// without requiring a rebuild in many workflows.
func openCityReader() (*maxminddb.Reader, error) {
	var lastErr error

	openOrNote := func(path string, r *maxminddb.Reader, err error) (*maxminddb.Reader, bool) {
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", path, err)
			return nil, false
		}
		return r, true
	}

	if env := os.Getenv("GEOIP_CITY_DB"); env != "" {
		fi, statErr := os.Stat(env)
		if statErr != nil {
			lastErr = fmt.Errorf("%s (GEOIP_CITY_DB): stat: %w", env, statErr)
		} else if fi.Size() == 0 {
			lastErr = fmt.Errorf("%s (GEOIP_CITY_DB) is empty (0 bytes)", env)
		} else {
			rDb, openErr := maxminddb.Open(env)
			if r, ok := openOrNote(env, rDb, openErr); ok {
				return r, nil
			}
		}
	}

	for _, p := range defaultCityPaths() {
		fi, statErr := os.Stat(p)
		if statErr != nil {
			continue
		}
		if fi.Size() == 0 {
			lastErr = fmt.Errorf("%s is empty (0 bytes); download GeoLite2 City (.mmdb) from MaxMind", p)
			continue
		}
		rDb, openErr := maxminddb.Open(p)
		if r, ok := openOrNote(p, rDb, openErr); ok {
			return r, nil
		}
	}

	if len(data.CityDB) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("failed to load City database: %w\n(hint: no embedded DB — place GeoLite2-City.mmdb in data/ or set GEOIP_CITY_DB)", lastErr)
		}
		return nil, fmt.Errorf(`failed to load City database: no GeoLite2-City.mmdb found (embed empty; add data/GeoLite2-City.mmdb or set GEOIP_CITY_DB)`)
	}

	r, err := maxminddb.OpenBytes(data.CityDB)
	if err != nil {
		hint := "embedded GeoLite2-City bytes are invalid; replace data/GeoLite2-City.mmdb with a real MMDB from MaxMind, run go build/run again, or set GEOIP_CITY_DB"
		if lastErr != nil {
			return nil, fmt.Errorf("failed to load City database: embedded: %w; also tried filesystem: %v\n(%s)", err, lastErr, hint)
		}
		return nil, fmt.Errorf("failed to load City database (embedded bytes): %w\n(%s)", err, hint)
	}
	return r, nil
}

func defaultCityPaths() []string {
	out := []string{filepath.Join("data", "GeoLite2-City.mmdb")}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(filepath.Clean(exe))
		out = append(out, filepath.Join(dir, "data", "GeoLite2-City.mmdb"))
	}
	return out
}

func (g *GeoIPLookup) LookupCity(ipStr string) (*CityInfo, error) {
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return nil, fmt.Errorf("invalid IP address: %w", err)
	}

	var record struct {
		Country struct {
			ISOCode string            `maxminddb:"iso_code"`
			Names   map[string]string `maxminddb:"names"`
		} `maxminddb:"country"`
		RegisteredCountry struct {
			ISOCode string            `maxminddb:"iso_code"`
			Names   map[string]string `maxminddb:"names"`
		} `maxminddb:"registered_country"`
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		Subdivisions []struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"subdivisions"`
	}

	err = g.cityReader.Lookup(ip).Decode(&record)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup city: %w", err)
	}

	info := &CityInfo{}

	if enName, ok := record.City.Names["en"]; ok {
		info.City = enName
	}

	// Anycast / CDN ranges often omit "country" but still set "registered_country".
	if enName, ok := record.Country.Names["en"]; ok && enName != "" {
		info.Country = enName
	} else if enName, ok := record.RegisteredCountry.Names["en"]; ok && enName != "" {
		info.Country = enName
	} else if record.Country.ISOCode != "" {
		info.Country = record.Country.ISOCode
	} else if record.RegisteredCountry.ISOCode != "" {
		info.Country = record.RegisteredCountry.ISOCode
	}

	if len(record.Subdivisions) > 0 {
		if enName, ok := record.Subdivisions[0].Names["en"]; ok {
			info.Province = enName
		}
	}

	return info, nil
}

func (g *GeoIPLookup) Close() {
	if g.cityReader != nil {
		g.cityReader.Close()
	}
}
