package release

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestLoadStages(t *testing.T) {
	stages, err := LoadStages([]byte(`
stages:
  - name: a
    gate: [{gateways-understand: 3}]
    check: [links-connected]
  - name: b
    requires: [a]
`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []Condition{{Check: "links-connected"}}; len(stages) != 2 || !slices.Equal(stages[0].Check, want) {
		t.Fatalf("stages %+v", stages)
	}
	if gate := stages[0].Gate[0]; gate != (Condition{Check: "gateways-understand", Level: 3}) || gate.String() != "gateways-understand: 3" {
		t.Fatalf("gate %+v", gate)
	}
}

func TestLoadStagesRefuses(t *testing.T) {
	for name, c := range map[string]struct{ stages, reason string }{
		"a requires after it":             {"[{name: a, requires: [b]}, {name: b}]", "requires b"},
		"an unknown requires":             {"[{name: a, requires: [z]}]", "requires z"},
		"an unknown check":                {"[{name: a, check: [all-gateways-current]}]", `"all-gateways-current"`},
		"an unknown gate":                 {"[{name: a, gate: [all-gateways-current]}]", `"all-gateways-current"`},
		"a level check without its level": {"[{name: a, gate: [gateways-understand]}]", `"gateways-understand"`},
		"a level of zero":                 {"[{name: a, gate: [{gateways-understand: 0}]}]", `"gateways-understand"`},
		"a level that is no number":       {"[{name: a, gate: [{gateways-understand: new}]}]", "int32"},
		"a level for a plain check":       {"[{name: a, check: [{nodes-ready: 2}]}]", `"nodes-ready"`},
		"two entries in one":              {"[{name: a, gate: [{gateways-understand: 1, nodes-ready: 1}]}]", "not one check"},
		"a name used twice":               {"[{name: a}, {name: a}]", "used twice"},
		"an unknown field":                {"[{name: a, rollout: hubs}]", "rollout"},
	} {
		if _, err := LoadStages([]byte("stages: " + c.stages)); err == nil || !strings.Contains(err.Error(), c.reason) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The controller's gate, and the check of the stage that places Station's agents, ask for the level of the document
// the controller of this tree writes. Station is a module this one does not import, so its constant is read from its
// source.
func TestShippedStages(t *testing.T) {
	stages, err := LoadStages(StagesYAML)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../../../astribot/station/network/compute.go")
	if err != nil {
		t.Fatal(err)
	}
	written := regexp.MustCompile(`(?m)^const DocumentLevel = (\d+)$`).FindSubmatch(source)
	for _, stage := range stages {
		for _, condition := range slices.Concat(stage.Gate, stage.Check) {
			if condition.Level > 0 && (written == nil || condition.String() != "gateways-understand: "+string(written[1])) {
				t.Errorf("stage %s asks for %s, the controller writes level %s", stage.Name, condition, written)
			}
		}
	}
}
