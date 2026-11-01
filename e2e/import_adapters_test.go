//go:build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestImportAdaptersUI(t *testing.T) {
	s := start(t, "import-ui", fmt.Sprintf(soa, "import-ui"))
	out, err := exec.Command("node", "import-adapters-ui.mjs", s.url("")).CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("import browser integration: %v", err)
	}
}
