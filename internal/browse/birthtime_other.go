//go:build !darwin && !windows

package browse

import (
	"os"
	"time"
)

// Modification/change time is not a substitute for an unavailable birth time.
func birthTime(_ os.FileInfo) time.Time { return time.Time{} }
