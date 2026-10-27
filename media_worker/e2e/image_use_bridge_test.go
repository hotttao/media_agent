package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// Keep the real Python bridge contracts in the normal Worker test gate. No
// browser, account, provider request or network access is used by this suite.
func TestImageUseBridgeContracts(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for GPT bridge contract tests")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source location unavailable")
	}
	tools := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "tools"))
	// Register external source reads with the Go test cache so a bridge-only
	// change cannot reuse a previous successful test result.
	for _, name := range []string{"image_use_bridge.py", "test_image_use_bridge.py"} {
		if _, err = os.ReadFile(filepath.Join(tools, name)); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command(python, "-m", "unittest", "discover", "-s", tools, "-p", "test_*.py")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("image-use bridge contracts failed: %v\n%s", err, output)
	}
}
