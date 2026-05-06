package data

import (
	_ "embed"
)

//go:embed GeoLite2-City.mmdb
var CityDB []byte
