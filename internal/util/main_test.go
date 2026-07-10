package util_test

import (
	"os"
	"testing"

	"github.com/WangYihang/Proxy-Verifier/internal"
	"github.com/WangYihang/Proxy-Verifier/internal/model"
)

// TestMain wires up the package-level Options that util reads (notably the
// shared secret used by the challenge/response verification) so the tests do
// not depend on the verifier's command-line initialization.
func TestMain(m *testing.M) {
	internal.Options = &model.MainOptions{}
	internal.Options.Secret = testSecret
	os.Exit(m.Run())
}
