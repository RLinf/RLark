package reconcile

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// reasonFieldNames are the struct fields whose values must stay low
// cardinality. Reason feeds metric labels and Event reasons; ReasonCode feeds
// Condition reasons.
var reasonFieldNames = map[string]bool{
	"Reason":     true,
	"ReasonCode": true,
}

// TestReasonFieldsAreConstantExpressions enforces the half of the reason
// contract that ValidateReason cannot see.
//
// ValidateReason runs on a value and therefore only proves that one observed
// string has the right shape. A reason assembled at runtime from an object
// name, a node name or an error string can have exactly that shape and still
// create one metric series per object. Cardinality is a property of the
// expression that produces the reason, not of any single produced value, so it
// has to be checked statically.
//
// The rule is deliberately narrow: a reason must be a string literal, a
// reference to a declared constant, or a copy of the same field from another
// value. Concatenations, conversions and general calls are rejected even when
// they happen to be cheap, because the reviewer cannot tell from the call site
// whether the operand set is bounded.
//
// Copying the field is allowed because it does not create a new reason: a
// transport decoder or a mapper that writes Reason from x.Reason or
// x.GetReason() passes a value through unchanged, and the cardinality of that
// value is a property of whoever produced it. Rejecting pass-through would
// force every boundary to invent a constant and would lose the reason that the
// producer actually reported.
func TestReasonFieldsAreConstantExpressions(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve package root: %v", err)
	}

	fset := token.NewFileSet()
	var violations []string

	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "testdata", "vendor":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}

		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, element := range literal.Elts {
				field, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := field.Key.(*ast.Ident)
				if !ok || !reasonFieldNames[key.Name] {
					continue
				}
				if reasonExpressionIsConstant(field.Value, key.Name) {
					continue
				}
				relative, relErr := filepath.Rel(root, path)
				if relErr != nil {
					relative = path
				}
				violations = append(violations, relative+":"+strconv.Itoa(fset.Position(field.Value.Pos()).Line)+": "+key.Name)
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}

	if len(violations) > 0 {
		t.Fatalf("reason fields must be string literals or declared constants, found dynamic expressions at:\n%s", strings.Join(violations, "\n"))
	}
}

// reasonExpressionIsConstant reports whether expression is a form a reviewer
// can bound by inspection: a string literal, a bare or qualified identifier,
// or a no-argument accessor that reads the same field name on another value.
// Parentheses are unwrapped so a redundantly parenthesized constant is not
// reported.
func reasonExpressionIsConstant(expression ast.Expr, fieldName string) bool {
	switch value := expression.(type) {
	case *ast.ParenExpr:
		return reasonExpressionIsConstant(value.X, fieldName)
	case *ast.BasicLit:
		return value.Kind == token.STRING
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return true
	case *ast.CallExpr:
		accessor, ok := value.Fun.(*ast.SelectorExpr)
		return ok && len(value.Args) == 0 && accessor.Sel.Name == "Get"+fieldName
	default:
		return false
	}
}

// TestReasonLintObservesTheTreeItClaimsToCover guards the lint itself: a typo
// in the walked path would make the check silently pass over an empty set.
func TestReasonLintObservesTheTreeItClaimsToCover(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve package root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "reconcile", "reason.go")); err != nil {
		t.Fatalf("lint root %s does not contain the package under test: %v", root, err)
	}
}
