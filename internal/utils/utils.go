package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

func GenFilenameFromMountPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absParts := strings.Split(absPath, "/")
	return fmt.Sprintf("zmount_%s", strings.Join(absParts, "_")), nil
}
