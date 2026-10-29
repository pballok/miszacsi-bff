package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const codeDepthLimitExceeded = "DEPTH_LIMIT_EXCEEDED"

// depthLimit rejects operations whose selection depth exceeds max. gqlgen has no built-in
// depth limit.
type depthLimit struct {
	max int
}

var (
	_ graphql.HandlerExtension        = depthLimit{}
	_ graphql.OperationContextMutator = depthLimit{}
)

func (depthLimit) ExtensionName() string { return "DepthLimit" }

func (depthLimit) Validate(graphql.ExecutableSchema) error { return nil }

func (d depthLimit) MutateOperationContext(_ context.Context, rc *graphql.OperationContext) *gqlerror.Error {
	if rc.Operation == nil {
		return nil
	}
	depth := selectionDepth(rc.Operation.SelectionSet, rc.Doc.Fragments, map[string]bool{})
	if depth <= d.max {
		return nil
	}
	return &gqlerror.Error{
		Message:    fmt.Sprintf("query depth %d exceeds limit %d", depth, d.max),
		Extensions: map[string]any{"code": codeDepthLimitExceeded},
	}
}

// selectionDepth returns the deepest field nesting in set. Fragments are inlined; visiting
// holds the fragments on the current path so cyclic spreads terminate. Meta fields such as
// __typename are not counted, and neither are introspection subtrees (__schema, __type), which
// are only reachable when introspection is enabled in development.
func selectionDepth(set ast.SelectionSet, fragments ast.FragmentDefinitionList, visiting map[string]bool) int {
	maxDepth := 0
	for _, sel := range set {
		var d int
		switch s := sel.(type) {
		case *ast.Field:
			if strings.HasPrefix(s.Name, "__") {
				continue
			}
			d = 1 + selectionDepth(s.SelectionSet, fragments, visiting)
		case *ast.InlineFragment:
			d = selectionDepth(s.SelectionSet, fragments, visiting)
		case *ast.FragmentSpread:
			if visiting[s.Name] {
				continue
			}
			def := fragments.ForName(s.Name)
			if def == nil {
				continue
			}
			visiting[s.Name] = true
			d = selectionDepth(def.SelectionSet, fragments, visiting)
			delete(visiting, s.Name)
		}
		maxDepth = max(maxDepth, d)
	}
	return maxDepth
}
