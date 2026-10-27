package operationlevel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
)

func TestSatisfies(t *testing.T) {
	type test struct {
		level    identity.Level
		required identity.Level

		expected bool
	}

	tests := map[string]func(t *testing.T) test{
		"account_satisfies_guest": func(t *testing.T) test {
			return test{level: identity.LevelAccount, required: identity.LevelGuest, expected: true}
		},
		"account_satisfies_account": func(t *testing.T) test {
			return test{level: identity.LevelAccount, required: identity.LevelAccount, expected: true}
		},
		"guest_satisfies_guest": func(t *testing.T) test {
			return test{level: identity.LevelGuest, required: identity.LevelGuest, expected: true}
		},
		"guest_does_not_satisfy_account": func(t *testing.T) test {
			return test{level: identity.LevelGuest, required: identity.LevelAccount, expected: false}
		},
		"anonymous_satisfies_anonymous": func(t *testing.T) test {
			return test{level: identity.LevelAnonymous, required: identity.LevelAnonymous, expected: true}
		},
		"anonymous_does_not_satisfy_guest": func(t *testing.T) test {
			return test{level: identity.LevelAnonymous, required: identity.LevelGuest, expected: false}
		},
		"unknown_level_satisfies_nothing": func(t *testing.T) test {
			return test{level: "admin", required: identity.LevelAnonymous, expected: false}
		},
		"unknown_requirement_is_never_satisfied": func(t *testing.T) test {
			return test{level: identity.LevelAccount, required: "admin", expected: false}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			require.Equal(t, tc.expected, Satisfies(tc.level, tc.required))
		})
	}
}

func TestCatalog(t *testing.T) {
	require.Equal(t, []identity.OperationLevel{
		{Operation: identity.OperationCreateSession, RequiredLevel: identity.LevelAccount},
		{Operation: identity.OperationJoinSessionAsAdmin, RequiredLevel: identity.LevelAccount},
		{Operation: identity.OperationJoinSessionAsPlayer, RequiredLevel: identity.LevelGuest},
		{Operation: identity.OperationReportClientTelemetry, RequiredLevel: identity.LevelGuest},
		{Operation: identity.OperationViewOwnIdentity, RequiredLevel: identity.LevelGuest},
	}, Catalog())
}

func TestRequiredKnowsOnlyCatalogedOperations(t *testing.T) {
	level, ok := Required(identity.OperationCreateSession)
	require.True(t, ok)
	require.Equal(t, identity.LevelAccount, level)

	_, ok = Required("DELETE_EVERYTHING")
	require.False(t, ok)
}

// TestEveryDeclaredOperationIsCataloged reads identity.go, so declaring an
// Operation without giving it a level fails here instead of being forbidden
// at run time.
func TestEveryDeclaredOperationIsCataloged(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../../../identity.go", nil, 0)
	require.NoError(t, err)

	var declared []identity.Operation
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			typeName, ok := value.Type.(*ast.Ident)
			if !ok || typeName.Name != "Operation" {
				continue
			}
			for _, expr := range value.Values {
				literal, ok := expr.(*ast.BasicLit)
				if !ok {
					require.Fail(t, "declare each Operation as `Name Operation = \"VALUE\"` so this test can check it")
				}
				name, err := strconv.Unquote(literal.Value)
				require.NoError(t, err)
				declared = append(declared, identity.Operation(name))
			}
		}
	}

	require.NotEmpty(t, declared)
	for _, operation := range declared {
		_, ok := Required(operation)
		require.True(t, ok, "operation %s has no required level in the catalog", operation)
	}
	require.Len(t, Catalog(), len(declared), "the catalog lists an operation identity.go does not declare")
}
