package parse

import (
	"path/filepath"
	"strings"
)

func PlayerNameFromLogPath(p string) (string, bool) {
	base := filepath.Base(p)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if !strings.HasPrefix(base, "eqlog_") {
		return "", false
	}
	rest := strings.TrimPrefix(base, "eqlog_")
	parts := strings.Split(rest, "_")
	if len(parts) < 2 {
		return "", false
	}
	if parts[0] == "" {
		return "", false
	}
	return parts[0], true
}
