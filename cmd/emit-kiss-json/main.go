// emit-kiss-json writes the definition of a KISS device, in the format of a
// KISS device definition file (see kiss.DeviceFile), to standard output.  It
// is a starting point for writing a definition file for another device.
//
// usage: emit-kiss-json «device-name»
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rothskeller/packet/v4/jnos/kiss"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: emit-kiss-json «device-name»")
		os.Exit(2)
	}
	d := kiss.GetDevice(os.Args[1])
	if d == nil {
		fmt.Fprintf(os.Stderr, "no such KISS device %q\n", os.Args[1])
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(d.File())
}
