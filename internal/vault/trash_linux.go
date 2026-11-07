package vault

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MoveToTrash moves a file or directory to the freedesktop.org Trash (Decision 10).
// It never performs a permanent delete. If trash is unavailable, it returns an actionable error.
func MoveToTrash(absPath string) error {
	if strings.TrimSpace(absPath) == "" {
		return fmt.Errorf("invalid path")
	}
	abs, err := filepath.Abs(absPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	if _, err := os.Lstat(abs); err != nil {
		return err
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("OS trash unavailable: cannot determine user home: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	filesDir := filepath.Join(dataHome, "Trash", "files")
	infoDir := filepath.Join(dataHome, "Trash", "info")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return fmt.Errorf("OS trash unavailable: cannot create %s: %w", filesDir, err)
	}
	if err := os.MkdirAll(infoDir, 0o700); err != nil {
		return fmt.Errorf("OS trash unavailable: cannot create %s: %w", infoDir, err)
	}
	base := filepath.Base(abs)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return fmt.Errorf("refusing to trash invalid path")
	}
	destName := uniqueTrashName(filesDir, infoDir, base)
	destPath := filepath.Join(filesDir, destName)
	infoPath := filepath.Join(infoDir, destName+".trashinfo")
	// Write .trashinfo first is not atomic with move; but if move fails we clean up.
	deletionDate := time.Now().Format("2006-01-02T15:04:05")
	escapedPath := url.PathEscape(abs)
	// PathEscape encodes '/', keep path separators readable as required by spec? spec says percent-encode.
	escapedPath = strings.ReplaceAll(escapedPath, "%2F", "/")
	// Keep leading slash encoded form stable.
	if strings.HasPrefix(abs, "/") && !strings.HasPrefix(escapedPath, "/") {
		escapedPath = "/" + escapedPath
	}
	content := "[Trash Info]\nPath=" + escapedPath + "\nDeletionDate=" + deletionDate + "\n"
	if err := os.WriteFile(infoPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("OS trash unavailable: cannot write trash metadata: %w", err)
	}
	if err := os.Rename(abs, destPath); err != nil {
		_ = os.Remove(infoPath)
		// Cross-filesystem rename fails; block with actionable error (Decision 10 fallback = block, no permanent delete).
		return fmt.Errorf("OS trash failed (move to trash directory not possible): %w. Permanent delete is not performed.", err)
	}
	return nil
}

func uniqueTrashName(filesDir, infoDir, base string) string {
	candidate := base
	for i := 1; i < 10000; i++ {
		_, e1 := os.Lstat(filepath.Join(filesDir, candidate))
		_, e2 := os.Lstat(filepath.Join(infoDir, candidate+".trashinfo"))
		if os.IsNotExist(e1) && os.IsNotExist(e2) {
			return candidate
		}
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		candidate = fmt.Sprintf("%s.%d%s", stem, i, ext)
	}
	return fmt.Sprintf("%s.%d", base, time.Now().UnixNano())
}
