//go:build (dragonfly && cgo) || (freebsd && cgo) || linux || netbsd || openbsd

package config

import (
	"errors"
	"net"
	"os/exec"
	"strings"

	"github.com/godbus/dbus/v5"
)

func noSecretService(err error) bool {
	if noSessionBus(err) {
		return true
	}
	dbusErr, ok := errors.AsType[dbus.Error](err)
	if !ok {
		return false
	}
	return dbusErr.Name == "org.freedesktop.DBus.Error.ServiceUnknown" ||
		strings.HasPrefix(dbusErr.Name, "org.freedesktop.DBus.Error.Spawn.")
}

func noSessionBus(err error) bool {
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	// dbus-launch is the only program go-keyring runs on these systems.
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return true
	}
	opErr, ok := errors.AsType[*net.OpError](err)
	return ok && opErr.Op == "dial"
}
