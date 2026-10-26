package browse

import (
	"os"
	"syscall"
	"time"
)

func birthTime(info os.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Birthtimespec != (syscall.Timespec{}) {
		return time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
	}
	return time.Time{}
}
