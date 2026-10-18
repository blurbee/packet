package kiss

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestBuiltinDevices checks that the built-in definitions are valid, and that
// the device-specific handling is confined to the devices that call for it.
func TestBuiltinDevices(t *testing.T) {
	for name, d := range devices {
		if err := d.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if d.Name != name {
			t.Errorf("%s is defined with Name %q", name, d.Name)
		}
	}
	// The generic TNC gets nothing but the standard KISS settings.
	g := devices["Generic KISS TNC"]
	if g.FullDuplex || g.AssertDTR || g.AssertRTS || g.Reset != "" || g.ResetSettle != 0 {
		t.Errorf("Generic KISS TNC has device-specific handling: %+v", g)
	}
	// The TH-D75's handling is in its definition.
	k := devices["Kenwood TH-D75"]
	if !k.FullDuplex || !k.AssertDTR || !k.AssertRTS || k.Reset != resetKenwood || k.ResetSettle == 0 {
		t.Errorf("Kenwood TH-D75 lacks its handling: %+v", k)
	}
}

// TestDeviceFileRoundTrip checks that a definition survives conversion to and
// from the definition file form.
func TestDeviceFileRoundTrip(t *testing.T) {
	for name, d := range devices {
		if got := d.File().Device(); *got != *d {
			t.Errorf("%s: round trip gave %+v, want %+v", name, *got, *d)
		}
	}
}

// TestDeviceFiles checks loading definition files: valid ones add or replace
// devices, and invalid ones are skipped.
func TestDeviceFiles(t *testing.T) {
	saved := maps.Clone(devices)
	t.Cleanup(func() { devices = saved })
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0666); err != nil {
			t.Fatal(err)
		}
		return path
	}
	// A new device, with names in any case.
	fetchOne(write("new.json", `{"Name": "My TNC", "pacLen": 64, "MAXFRAME": 1, "frackMs": 5000,
		"retry": 4, "respTimeMs": 300, "turnaroundMs": 1000, "assertDTR": true}`))
	d := devices["My TNC"]
	if d == nil {
		t.Fatal("valid definition not loaded")
	}
	if d.PacLen != 64 || d.MaxFrame != 1 || d.Frack != 5*time.Second || d.Turnaround != time.Second || !d.AssertDTR || d.AssertRTS || d.Reset != "" {
		t.Errorf("loaded %+v", *d)
	}
	// Replacing a built-in definition.
	fetchOne(write("override.json", `{"name": "Generic KISS TNC", "pacLen": 200, "maxFrame": 3, "frackMs": 4000,
		"retry": 10, "respTimeMs": 500, "turnaroundMs": 1500}`))
	if devices["Generic KISS TNC"].PacLen != 200 {
		t.Error("definition file didn't replace the built-in definition")
	}
	// Invalid definitions are skipped.
	for name, content := range map[string]string{
		"badreset.json": `{"name": "Bad 1", "pacLen": 64, "maxFrame": 1, "frackMs": 5000, "retry": 4, "respTimeMs": 300, "turnaroundMs": 1000, "reset": "nosuch"}`,
		"unknown.json":  `{"name": "Bad 2", "pacLen": 64, "maxFrame": 1, "frackMs": 5000, "retry": 4, "respTimeMs": 300, "turnaroundMs": 1000, "colour": "red"}`,
		"range.json":    `{"name": "Bad 3", "pacLen": 999, "maxFrame": 1, "frackMs": 5000, "retry": 4, "respTimeMs": 300, "turnaroundMs": 1000}`,
		"noname.json":   `{"pacLen": 64, "maxFrame": 1, "frackMs": 5000, "retry": 4}`,
		"syntax.json":   `{"name": "Bad 4", `,
	} {
		fetchOne(write(name, content))
	}
	for _, name := range []string{"Bad 1", "Bad 2", "Bad 3", "Bad 4", ""} {
		if devices[name] != nil {
			t.Errorf("invalid definition %q was loaded", name)
		}
	}
}
