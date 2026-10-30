//go:build unix

package config

import (
	"io/fs"
	"os"
	"syscall"

	"chronos.sh/cli/internal/exit"
)

// checkPrivate refuses a path that another user could replace.
func checkPrivate(path string, info fs.FileInfo) error {
	if info.Mode().Perm()&0o022 != 0 {
		return exit.Failuref("%s can be changed by other users, so chronos will not use it. Run chmod go-w %s, or set CHRONOS_CONFIG_DIR to a directory of your own.", path, path)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid == 0 || int(st.Uid) == os.Getuid() {
		return nil
	}
	return exit.Failuref("%s belongs to another user, so chronos will not use it. Set CHRONOS_CONFIG_DIR to a directory of your own.", path)
}

func checkOthersCannotRead(path string, info fs.FileInfo) error {
	if info.Mode().Perm()&0o044 == 0 {
		return nil
	}
	return exit.Failuref("%s holds your API key and other users can read it, so chronos will not use it. Run chmod 600 %s.", path, path)
}
