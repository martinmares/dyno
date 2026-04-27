package sitepath

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	RootDir    string
	ContentDir string
}

func Resolve(input string) (Paths, error) {
	abs, err := filepath.Abs(input)
	if err != nil {
		return Paths{}, fmt.Errorf("invalid site path %q: %w", input, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return Paths{}, err
	}
	if !info.IsDir() {
		return Paths{}, fmt.Errorf("site path is not a directory: %s", abs)
	}

	return Paths{
		RootDir:    filepath.Dir(abs),
		ContentDir: abs,
	}, nil
}
