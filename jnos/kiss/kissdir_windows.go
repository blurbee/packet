//go:build windows

package kiss

// KISSDir returns the pathname of the directory that contains KISS device
// definition files (see DeviceFile), beside the TNC definitions directory.
func KISSDir() string {
	return `C:\PackItForms\KISS`
}
