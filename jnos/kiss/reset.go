package kiss

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// A resetPort is the connection to the radio used for resetting its TNC.
// (go.bug.st/serial's Port satisfies it.)
type resetPort interface {
	io.ReadWriteCloser
	SetReadTimeout(time.Duration) error
}

// A resetMethod resets a TNC to a clean state over port (which is open only
// for the purpose), discarding any frames it still holds, and leaves it ready
// for KISS again.  It returns a description of what it did, for the monitor
// (e.g., "left KISS mode, ... and re-entered it with TN 2,0").  It must send
// nothing that transmits.  Its errors should say how to recover by hand.
type resetMethod func(port resetPort) (did string, err error)

// resetMethods are the methods for resetting TNCs, by the name a device
// definition uses for them in its Reset field.
var resetMethods = map[string]resetMethod{
	resetKenwood: kenwoodReset,
}

var errNoAnswer = errors.New("no answer")

// drain discards anything the radio has already sent.
func drain(port resetPort) {
	buf := make([]byte, 256)
	for range 16 {
		if port.SetReadTimeout(50*time.Millisecond) != nil {
			return
		}
		if count, err := port.Read(buf); count == 0 || err != nil {
			return
		}
	}
}

// sessionMarker returns the name of the file whose existence marks a session
// in progress on serialPort, or "" if there's nowhere to put it.  The file is
// created when a session starts, and removed when it ends with the TNC in good
// order.  If it's there when a session starts, the last one never finished
// (the program was killed, or the computer crashed or slept, in the middle of
// it), and the TNC may still hold its frames.
func sessionMarker(serialPort string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	name := []byte(filepath.Base(serialPort))
	for i, b := range name {
		if !('a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9' || b == '.' || b == '-') {
			name[i] = '_'
		}
	}
	return filepath.Join(dir, "packet", "kiss-session-"+string(name))
}

// markSession creates the session marker file (see sessionMarker).  It's best
// effort: without it, we just can't tell later that the session never ended.
func markSession(marker string) {
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err == nil {
		err = os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644)
		if err != nil {
			slog.Warn("KISS session marker", "err", err)
		}
	}
}

// clearSession removes the session marker file (see sessionMarker).
func clearSession(marker string) {
	if marker != "" {
		os.Remove(marker)
	}
}

// resetBeforeStart resets the TNC on the (just opened) port with reset,
// because the last session on it never finished.  It reports to the monitor.
// If the reset fails, the TNC may not be ready for KISS, so it returns an
// error saying so rather than letting the connection fail obscurely.
func resetBeforeStart(port resetPort, reset resetMethod, monitor func(string)) error {
	monitorNote(monitor, "The last session on this port didn't finish; resetting the TNC first.")
	did, err := reset(port)
	if err != nil {
		slog.Warn("TNC reset before start", "err", err)
		monitorNote(monitor, fmt.Sprintf("TNC reset failed: %s.", err))
		return fmt.Errorf("the TNC needed a reset, which failed (%s); then try again", err)
	}
	slog.Info("TNC reset before start", "did", did)
	monitorNote(monitor, "TNC reset: "+did)
	return nil
}
