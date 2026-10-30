//go:build !unix

package config

import "io/fs"

// checkPrivate trusts the path, since mode bits do not describe Windows ACLs.
func checkPrivate(string, fs.FileInfo) error {
	return nil
}

func checkOthersCannotRead(string, fs.FileInfo) error {
	return nil
}
