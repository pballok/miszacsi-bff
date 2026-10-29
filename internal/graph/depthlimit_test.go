package graph

import (
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestDepthLimit(t *testing.T) {
	schema := NewExecutableSchema(Config{}).Schema()
	// reservations(1) > serviceType(2) > code(3)
	const depth3 = `{ reservations(from: "2030-01-15T10:00:00+01:00", to: "2030-01-16T10:00:00+01:00") {
		serviceType { code } } }`

	tests := []struct {
		name    string
		max     int
		query   string
		wantErr bool
	}{
		{"exactly at limit", 3, depth3, false},
		{"one over limit", 2, depth3, true},
		{
			name: "named fragment counted",
			max:  2,
			query: `query { reservations(from: "2030-01-15T10:00:00+01:00", to: "2030-01-16T10:00:00+01:00") { ...E } }
				fragment E on Reservation { serviceType { code } }`,
			wantErr: true,
		},
		{
			name: "nested named fragments at limit",
			max:  3,
			query: `query { reservations(from: "2030-01-15T10:00:00+01:00", to: "2030-01-16T10:00:00+01:00") { ...E } }
				fragment E on Reservation { serviceType { ...N } }
				fragment N on ServiceType { code }`,
			wantErr: false,
		},
		{
			name: "inline fragment counted",
			max:  2,
			query: `{ reservations(from: "2030-01-15T10:00:00+01:00", to: "2030-01-16T10:00:00+01:00") {
				... on Reservation { serviceType { code } } } }`,
			wantErr: true,
		},
		{
			name: "__typename ignored",
			max:  3,
			query: `{ reservations(from: "2030-01-15T10:00:00+01:00", to: "2030-01-16T10:00:00+01:00") {
				serviceType { code __typename } } }`,
			wantErr: false,
		},
		{
			name:    "introspection subtree ignored",
			max:     1,
			query:   `{ __schema { types { fields { type { ofType { name } } } } } }`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, errs := gqlparser.LoadQueryWithRules(schema, tt.query, nil)
			if errs != nil {
				t.Fatalf("parse query: %v", errs)
			}
			rc := &graphql.OperationContext{Doc: doc, Operation: doc.Operations[0]}
			gqlErr := depthLimit{max: tt.max}.MutateOperationContext(t.Context(), rc)
			if (gqlErr != nil) != tt.wantErr {
				t.Fatalf("MutateOperationContext() = %v, wantErr %v", gqlErr, tt.wantErr)
			}
			if gqlErr != nil && gqlErr.Extensions["code"] != codeDepthLimitExceeded {
				t.Errorf("code = %v, want %s", gqlErr.Extensions["code"], codeDepthLimitExceeded)
			}
		})
	}
}

func TestSelectionDepthFragmentCycle(t *testing.T) {
	// Validation rejects cyclic fragments before the limiter runs; build the AST by hand to prove
	// the walker still terminates.
	cycle := &ast.FragmentDefinition{Name: "A"}
	cycle.SelectionSet = ast.SelectionSet{
		&ast.Field{Name: "serviceType", SelectionSet: ast.SelectionSet{&ast.Field{Name: "code"}}},
		&ast.FragmentSpread{Name: "A"},
	}
	set := ast.SelectionSet{&ast.Field{Name: "reservations", SelectionSet: ast.SelectionSet{&ast.FragmentSpread{Name: "A"}}}}
	if got := selectionDepth(set, ast.FragmentDefinitionList{cycle}, map[string]bool{}); got != 3 {
		t.Errorf("selectionDepth() = %d, want 3", got)
	}
}
