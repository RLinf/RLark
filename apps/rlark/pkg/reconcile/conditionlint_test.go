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

// conditionContract is the generation contract a condition type carries, as
// declared by api/rlark.io/v1alpha1/conditions.go.
type conditionContract int

const (
	conditionContractUnknown conditionContract = iota
	// conditionContractSpecDriven conditions answer a question about a
	// specific spec revision, so the revision they answered for is part of
	// the answer.
	conditionContractSpecDriven
	// conditionContractObservationDriven conditions answer a question about
	// the world, which no spec revision can date.
	conditionContractObservationDriven
)

// conditionContractsByConstName classifies the condition types by the name of
// the constant that declares them. The constants live in the separate api
// module, which the walked tree cannot see, so the classification is keyed by
// identifier rather than resolved through the type checker.
var conditionContractsByConstName = map[string]conditionContract{
	"ConditionTypeValidated":   conditionContractSpecDriven,
	"ConditionTypeProgressing": conditionContractSpecDriven,
	"ConditionTypeReady":       conditionContractObservationDriven,
	"ConditionTypeDegraded":    conditionContractObservationDriven,
}

// conditionContractsByValue classifies the same condition types by the string
// they carry, so that a hand-written literal cannot escape the rule by
// bypassing the constant.
var conditionContractsByValue = map[string]conditionContract{
	"Validated":   conditionContractSpecDriven,
	"Progressing": conditionContractSpecDriven,
	"Ready":       conditionContractObservationDriven,
	"Degraded":    conditionContractObservationDriven,
}

// TestConditionLiteralsHonourTheirGenerationContract enforces the half of the
// condition contract that no runtime check can enforce.
//
// ApplyConditionMutations backfills ObservedGeneration from the generation it
// is handed, so a spec-driven condition written without one still ends up with
// a plausible value. That makes the omission invisible at runtime and wrong in
// exactly the case that matters: the backfilled generation is whatever the
// caller passed, not the revision the condition was actually computed against,
// so a mutation built from a stale object and applied under a newer generation
// silently claims to have judged the newer spec. Treating the backfill as a
// safety net rather than as the normal path keeps the claim honest, and the
// only way to require that is to read the call site.
//
// The mirrored rule is just as load-bearing. An observation-driven condition
// that carries a generation invites consumers to wait for it to catch up with
// metadata.generation, which it never will, because nothing about a heartbeat
// or a reported disk pressure is tied to a spec revision. A condition that can
// never satisfy the staleness test readers apply to it is worse than one that
// declines to claim a generation at all.
//
// The check is a literal-level one, which is sound here because every
// condition in the tree is built as a composite literal; see
// TestConditionGenerationLintIsNotVacuous for the guard on that assumption.
func TestConditionLiteralsHonourTheirGenerationContract(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve package root: %v", err)
	}

	fset := token.NewFileSet()
	var violations []string

	_, walkErr := walkConditionLiterals(root, fset, func(path string, finding conditionLiteralFinding) {
		message := finding.violation()
		if message == "" {
			return
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			relative = path
		}
		violations = append(violations, relative+":"+strconv.Itoa(fset.Position(finding.Position).Line)+": "+message)
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}

	if len(violations) > 0 {
		t.Fatalf("conditions must declare ObservedGeneration exactly when their type is spec-driven, found:\n%s", strings.Join(violations, "\n"))
	}
}

// TestConditionGenerationLintIsNotVacuous guards the lint itself. A wrong walk
// root, a renamed constant or a move away from composite literals would all
// leave the check passing over an empty set, so the tree is required to still
// contain at least one condition of each kind.
func TestConditionGenerationLintIsNotVacuous(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve package root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "reconcile", "status.go")); err != nil {
		t.Fatalf("lint root %s does not contain the package under test: %v", root, err)
	}

	counts, walkErr := walkConditionLiterals(root, token.NewFileSet(), nil)
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}
	if counts[conditionContractSpecDriven] == 0 {
		t.Error("lint found no spec-driven condition literals; the rule is no longer observing anything")
	}
	if counts[conditionContractObservationDriven] == 0 {
		t.Error("lint found no observation-driven condition literals; the rule is no longer observing anything")
	}
}

// TestConditionGenerationLintFlagsBothDirections proves the rule rejects what
// it claims to reject. A lint that only ever runs over compliant code cannot
// distinguish "no violations" from "no detection".
func TestConditionGenerationLintFlagsBothDirections(t *testing.T) {
	const source = `package example

const localValidated = rlarkv1alpha1.ConditionTypeValidated
const indirectValidated = localValidated

func build() []any {
	return []any{
		metav1.Condition{Type: rlarkv1alpha1.ConditionTypeValidated, ObservedGeneration: job.Generation},
		metav1.Condition{Type: rlarkv1alpha1.ConditionTypeValidated},
		metav1.Condition{Type: localValidated},
		metav1.Condition{Type: indirectValidated},
		metav1.Condition{Type: "Progressing"},
		metav1.Condition{Type: rlarkv1alpha1.ConditionTypeReady},
		metav1.Condition{Type: rlarkv1alpha1.ConditionTypeDegraded, ObservedGeneration: node.Generation},
		metav1.Condition{Type: "Degraded", ObservedGeneration: 0},
		metav1.Condition{Type: somethingElse},
		sharedreconcile.ConditionMutation{Condition: metav1.Condition{Type: rlarkv1alpha1.ConditionTypeProgressing}, Remove: true},
		sharedreconcile.ConditionMutation{Condition: metav1.Condition{Type: rlarkv1alpha1.ConditionTypeProgressing}, Remove: false},
	}
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "example.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	var reported []string
	for _, finding := range findConditionLiterals(file) {
		if message := finding.violation(); message != "" {
			reported = append(reported, strconv.Itoa(fset.Position(finding.Position).Line)+": "+message)
		}
	}

	expected := []string{
		"9: spec-driven condition Validated must set ObservedGeneration",
		"10: spec-driven condition Validated must set ObservedGeneration",
		"11: spec-driven condition Validated must set ObservedGeneration",
		"12: spec-driven condition Progressing must set ObservedGeneration",
		"14: observation-driven condition Degraded must not set ObservedGeneration",
		"15: observation-driven condition Degraded must not set ObservedGeneration",
		"18: spec-driven condition Progressing must set ObservedGeneration",
	}
	if strings.Join(reported, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("lint findings differ\nwant:\n%s\ngot:\n%s", strings.Join(expected, "\n"), strings.Join(reported, "\n"))
	}
}

// conditionAlias is a file-local constant that forwards a condition type,
// carrying both the contract it inherits and the wire name to report so that a
// diagnostic names the condition rather than the local spelling of it.
type conditionAlias struct {
	Contract conditionContract
	TypeName string
}

// conditionLiteralFinding is one composite literal that carries a recognised
// condition type, together with whether it declared a generation.
type conditionLiteralFinding struct {
	Contract       conditionContract
	TypeName       string
	SetsGeneration bool
	// Position points at the field that has to change: the type for a
	// missing generation, the generation for a surplus one.
	Position token.Pos
}

// violation returns the reason this literal breaks the contract, or the empty
// string when it honours it.
func (f conditionLiteralFinding) violation() string {
	switch f.Contract {
	case conditionContractSpecDriven:
		if !f.SetsGeneration {
			return "spec-driven condition " + f.TypeName + " must set ObservedGeneration"
		}
	case conditionContractObservationDriven:
		if f.SetsGeneration {
			return "observation-driven condition " + f.TypeName + " must not set ObservedGeneration"
		}
	}
	return ""
}

// walkConditionLiterals parses every non-test Go file under root, reports each
// recognised condition literal to report when it is non-nil, and returns how
// many literals of each contract it saw.
func walkConditionLiterals(root string, fset *token.FileSet, report func(path string, finding conditionLiteralFinding)) (map[conditionContract]int, error) {
	counts := map[conditionContract]int{}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
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
		for _, finding := range findConditionLiterals(file) {
			counts[finding.Contract]++
			if report != nil {
				report(path, finding)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return counts, nil
}

// findConditionLiterals returns every composite literal in file whose Type
// field resolves to a known condition type. The literal's own type is
// deliberately not inspected: doing so would tie the lint to an import alias
// and would miss a condition built through a type alias, while the Type field
// alone already identifies the contract being claimed. Literals that only name
// a condition in order to withdraw it are exempt; see conditionRemovalLiterals.
func findConditionLiterals(file *ast.File) []conditionLiteralFinding {
	aliases := conditionTypeAliases(file)
	removals := conditionRemovalLiterals(file)

	var findings []conditionLiteralFinding
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if removals[literal] {
			return true
		}

		finding := conditionLiteralFinding{Contract: conditionContractUnknown}
		var generationPosition token.Pos
		for _, element := range literal.Elts {
			field, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := field.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch key.Name {
			case "Type":
				contract, name := classifyConditionType(field.Value, aliases)
				finding.Contract = contract
				finding.TypeName = name
				finding.Position = field.Key.Pos()
			case "ObservedGeneration":
				finding.SetsGeneration = true
				generationPosition = field.Key.Pos()
			}
		}
		if finding.Contract == conditionContractUnknown {
			return true
		}
		if finding.SetsGeneration && finding.Contract == conditionContractObservationDriven {
			finding.Position = generationPosition
		}
		findings = append(findings, finding)
		return true
	})
	return findings
}

// conditionRemovalLiterals returns the condition literals that exist only to
// name a condition being withdrawn. A ConditionMutation with Remove set to
// true carries nothing but the type, because RemoveStatusCondition matches on
// the type alone; demanding a generation there would force callers to invent a
// value that is never written.
func conditionRemovalLiterals(file *ast.File) map[*ast.CompositeLit]bool {
	removals := map[*ast.CompositeLit]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		mutation, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}

		var condition *ast.CompositeLit
		removed := false
		for _, element := range mutation.Elts {
			field, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := field.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch key.Name {
			case "Condition":
				condition, _ = field.Value.(*ast.CompositeLit)
			case "Remove":
				value, ok := field.Value.(*ast.Ident)
				removed = ok && value.Name == "true"
			}
		}
		if removed && condition != nil {
			removals[condition] = true
		}
		return true
	})
	return removals
}

// conditionTypeAliases maps file-local constants that forward a condition type
// to the contract they inherit. A controller that names the condition it owns
// is still bound by the contract of the type it points at, so the alias has to
// be followed rather than ignored. Aliases of aliases are resolved by
// repeating the pass until it stops making progress.
func conditionTypeAliases(file *ast.File) map[string]conditionAlias {
	aliases := map[string]conditionAlias{}

	for progressed := true; progressed; {
		progressed = false
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.CONST {
				continue
			}
			for _, specification := range generic.Specs {
				values, ok := specification.(*ast.ValueSpec)
				if !ok || len(values.Names) != 1 || len(values.Values) != 1 {
					continue
				}
				name := values.Names[0].Name
				if _, known := aliases[name]; known {
					continue
				}
				contract, typeName := classifyConditionType(values.Values[0], aliases)
				if contract == conditionContractUnknown {
					continue
				}
				aliases[name] = conditionAlias{Contract: contract, TypeName: typeName}
				progressed = true
			}
		}
	}
	return aliases
}

// classifyConditionType resolves a Type expression to its contract and to the
// condition name to report, which is the condition's wire name rather than the
// identifier spelled at the call site.
func classifyConditionType(expression ast.Expr, aliases map[string]conditionAlias) (conditionContract, string) {
	switch value := expression.(type) {
	case *ast.ParenExpr:
		return classifyConditionType(value.X, aliases)
	case *ast.SelectorExpr:
		return conditionContractsByConstName[value.Sel.Name], strings.TrimPrefix(value.Sel.Name, "ConditionType")
	case *ast.Ident:
		if contract, ok := conditionContractsByConstName[value.Name]; ok {
			return contract, strings.TrimPrefix(value.Name, "ConditionType")
		}
		alias := aliases[value.Name]
		return alias.Contract, alias.TypeName
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return conditionContractUnknown, ""
		}
		unquoted, err := strconv.Unquote(value.Value)
		if err != nil {
			return conditionContractUnknown, ""
		}
		return conditionContractsByValue[unquoted], unquoted
	default:
		return conditionContractUnknown, ""
	}
}
