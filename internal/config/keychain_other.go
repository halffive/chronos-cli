//go:build !((dragonfly && cgo) || (freebsd && cgo) || linux || netbsd || openbsd)

package config

func noSecretService(error) bool {
	return false
}
