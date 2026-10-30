//go:build (dragonfly && cgo) || (freebsd && cgo) || linux || netbsd || openbsd

package config_test

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"syscall"
	"testing"

	"github.com/godbus/dbus/v5"

	"chronos.sh/cli/internal/config"
)

func TestSecretServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		noKeychain bool
	}{
		{"dbus-launch not installed", &exec.Error{Name: "dbus-launch", Err: exec.ErrNotFound}, true},
		{"dbus-launch cannot start a bus", &exec.ExitError{}, true},
		{"nothing at the bus address", &net.OpError{Op: "dial", Net: "unix", Err: syscall.ENOENT}, true},
		{"no provider on the bus", dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"}, true},
		{"provider failed to start", dbus.Error{Name: "org.freedesktop.DBus.Error.Spawn.ChildExited"}, true},
		{"provider did not reply", dbus.Error{Name: "org.freedesktop.DBus.Error.NoReply"}, false},
		{"access denied", dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"}, false},
		{"bus connection dropped", &net.OpError{Op: "read", Net: "unix", Err: syscall.ECONNRESET}, false},
		{"unlock dismissed", fmt.Errorf("failed to unlock correct collection '/org/freedesktop/secrets/collection/login'"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errors.Is(config.KeychainError(tt.err), config.ErrNoKeychain); got != tt.noKeychain {
				t.Errorf("no keychain = %v, want %v", got, tt.noKeychain)
			}
		})
	}
}
