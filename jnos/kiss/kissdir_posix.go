//go:build !windows

package kiss

import (
	"os"
	"path/filepath"
)

// KISSDir returns the pathname of the directory that contains KISS device
// definition files (see DeviceFile), beside the TNC definitions directory.
func KISSDir() string {
	var home = os.Getenv("HOME")

	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "packet", "KISS")
}
