package listoperations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
)

func TestListOperations(t *testing.T) {
	operations := New().ListOperations(context.Background())

	require.Contains(t, operations, identity.OperationLevel{Operation: identity.OperationCreateSession, RequiredLevel: identity.LevelAccount})
	require.Contains(t, operations, identity.OperationLevel{Operation: identity.OperationJoinSessionAsPlayer, RequiredLevel: identity.LevelGuest})
	for i := 1; i < len(operations); i++ {
		require.Less(t, operations[i-1].Operation, operations[i].Operation)
	}
}
