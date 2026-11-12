//go:build darwin

package vault

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// macOS has no freedesktop trash, so use NSFileManager via osascript's
// JavaScript bridge. This hands the item to the OS trash and never performs a
// permanent delete; failures are surfaced as actionable errors (Decision 10).
const trashScript = `ObjC.import("Foundation");
function run(argv) {
  var url = $.NSURL.fileURLWithPath(argv[0]);
  var fm = $.NSFileManager.defaultManager;
  var err = $();
  var ok = fm.trashItemAtURLResultingItemURLError(url, $(), err);
  if (!ok) {
    throw new Error(err && err.localizedDescription ? err.localizedDescription.js : "move to Trash failed");
  }
  return "ok";
}`

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
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", trashScript, "--", abs).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("OS trash unavailable: %s. Permanent delete is not performed", msg)
	}
	return nil
}
