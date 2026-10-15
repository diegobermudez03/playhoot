package sessionlifecycle

import (
	"os"
	"testing"

	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}
