package browse

import (
	"os"
	"syscall"
	"time"
)

func birthTime(info os.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Win32FileAttributeData); ok && stat.CreationTime != (syscall.Filetime{}) {
		return time.Unix(0, stat.CreationTime.Nanoseconds())
	}
	return time.Time{}
}
