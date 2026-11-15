package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// migration20261115000000IdentityQueue creates Identity's queue table, where
// its asynchronous and periodic work waits.
func migration20261115000000IdentityQueue() *gormigrate.Migration {
	return txqueue.Migration("20261115000000_identity_queue", identity.QueueDomain)
}
