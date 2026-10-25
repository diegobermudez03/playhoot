// Package sessionlifetime defines how long a Session may exist.
package sessionlifetime

import "time"

// Max is the most time a Session may live from its creation, whatever its
// activity; a platform tuning value.
const Max = 4 * time.Hour
