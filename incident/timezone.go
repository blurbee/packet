package incident

import (
	"cmp"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The program reads times from the user, shows times to the user, and stores
// some times (such as the operational period) without a time zone, all in Go's
// local time zone, time.Local.  By default, that is the system's time zone.
// The TimeZone setting in the incident defaults, which applies to all
// incidents, replaces it.

var (
	// systemZone is the system's time zone.
	systemZone = time.Local
	// zoneName is the name of the time zone in effect, or "" if it is the
	// system's.  zoneMutex guards it, and our changes to time.Local.
	zoneName  string
	zoneMutex sync.Mutex
)

// TimeZone returns the name of the time zone in effect (an IANA time zone name
// such as "America/Los_Angeles"), or "" if it is the system's time zone.
func TimeZone() string {
	zoneMutex.Lock()
	defer zoneMutex.Unlock()
	return zoneName
}

// SystemTimeZone returns a name for the system's time zone, for display.  It
// is the IANA name if that can be found, or the zone's current abbreviation
// (e.g., "PDT") if not.
func SystemTimeZone() string {
	if tz, ok := os.LookupEnv("TZ"); ok && strings.TrimPrefix(tz, ":") != "" {
		return strings.TrimPrefix(tz, ":")
	}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if _, name, ok := strings.Cut(filepath.ToSlash(link), "zoneinfo/"); ok && name != "" {
			return name
		}
	}
	abbr, _ := time.Now().In(systemZone).Zone()
	return abbr
}

// LoadTimeZone returns the time zone with the specified IANA name, or the
// system's time zone if the name is empty (or "Local").
func LoadTimeZone(name string) (*time.Location, error) {
	if name = normalizeZone(name); name == "" {
		return systemZone, nil
	}
	if loc, err := time.LoadLocation(name); err == nil {
		return loc, nil
	}
	return nil, fmt.Errorf("%q is not a known time zone; use an IANA time zone name such as America/Los_Angeles", name)
}

func normalizeZone(name string) string {
	if name = strings.TrimSpace(name); strings.EqualFold(name, "Local") {
		return ""
	}
	return name
}

// ApplyTimeZone puts the time zone saved in the incident defaults into
// effect.  It should be called at startup, before anything reads the time.  If
// the saved zone can't be loaded, the system's time zone stays in effect.
func ApplyTimeZone() {
	if err := setLocal(GetIncDefaults().TimeZone); err != nil {
		slog.Error("time zone", "err", err)
	}
}

// SetTimeZone changes the time zone, for all incidents, and saves it in the
// incident defaults.  An empty name means the system's time zone.
func SetTimeZone(name string) (err error) {
	name = normalizeZone(name)
	if name == TimeZone() {
		return nil
	}
	if err = setLocal(name); err != nil {
		return err
	}
	updateIncDefaults(func(idef *IncDefaults) { idef.TimeZone = name })
	return nil
}

// setLocal makes the named time zone (or the system's, if name is empty) the
// local time zone.
func setLocal(name string) error {
	name = normalizeZone(name)
	loc, err := LoadTimeZone(name)
	if err != nil {
		return err
	}
	zoneMutex.Lock()
	defer zoneMutex.Unlock()
	if name == zoneName {
		return nil
	}
	// Times made before this keep the zone they were made in; everything
	// that uses the local time zone from now on uses this one.
	time.Local, zoneName = loc, name
	slog.Info("time zone", "zone", cmp.Or(name, "system ("+SystemTimeZone()+")"))
	return nil
}
