//go:build tiny

package data

// CityDB is intentionally empty in the tiny build.
// The MMDB must be provided via a filesystem path at runtime.
var CityDB []byte
