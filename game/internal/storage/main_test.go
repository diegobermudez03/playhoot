package repo

import (
	"os"
	"testing"

	"github.com/diegobermudez03/playhoot/game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}
