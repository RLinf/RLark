package release

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"

	"sigs.k8s.io/yaml"
)

// StagesYAML is the order of this build's batch. It is in the binary so that
// the checks a stage names are the ones this build has.
//
//go:embed stages.yaml
var StagesYAML []byte

// Stage is one step of a batch; stages.yaml describes the fields.
type Stage struct {
	Name     string      `json:"name"`
	Requires []string    `json:"requires,omitempty"`
	Changes  []string    `json:"changes,omitempty"`
	Gate     []Condition `json:"gate,omitempty"`
	Check    []Condition `json:"check,omitempty"`
	Note     string      `json:"note,omitempty"`
	// Recreates are the owners ("<namespace>/<owner>", path patterns) whose
	// Pods the stage replaces; any other Pod made anew stops the run.
	Recreates []string `json:"recreates,omitempty"`
	// Back: the stage is walked when a run goes back too.
	Back bool `json:"back,omitempty"`
	// TimeoutFactor is how many times the run's timeout the stage may take.
	TimeoutFactor int `json:"timeoutFactor,omitempty"`
}

// Condition is a check of Checks by name, or a check of LevelChecks with the level it is asked for.
type Condition struct {
	Check string
	Level int32
}

func (c *Condition) UnmarshalJSON(data []byte) error {
	if json.Unmarshal(data, &c.Check) == nil {
		return nil
	}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(data, &entry); err != nil {
		return err
	}
	if len(entry) != 1 {
		return fmt.Errorf("%s is not one check with its level", data)
	}
	for name, value := range entry {
		c.Check = name
		return json.Unmarshal(value, &c.Level)
	}
	return nil
}

func (c Condition) String() string {
	if c.Level > 0 {
		return fmt.Sprintf("%s: %d", c.Check, c.Level)
	}
	return c.Check
}

// LoadStages reads a stages file: its stages in the file's order, each after every stage it requires.
func LoadStages(data []byte) ([]Stage, error) {
	var file struct {
		Stages []Stage `json:"stages"`
	}
	if err := yaml.UnmarshalStrict(data, &file); err != nil {
		return nil, err
	}
	before := map[string]bool{}
	for _, stage := range file.Stages {
		if stage.Name == "" || before[stage.Name] {
			return nil, fmt.Errorf("stage name %q is empty or used twice", stage.Name)
		}
		for _, required := range stage.Requires {
			if !before[required] {
				return nil, fmt.Errorf("stage %s requires %s, which is not a stage before it", stage.Name, required)
			}
		}
		for _, condition := range slices.Concat(stage.Gate, stage.Check) {
			_, plain := Checks[condition.Check]
			_, levelled := LevelChecks[condition.Check]
			if !(plain && condition.Level == 0) && !(levelled && condition.Level > 0) {
				return nil, fmt.Errorf("stage %s names %q, which is neither a check nor a check with its level", stage.Name, condition.Check)
			}
		}
		before[stage.Name] = true
	}
	return file.Stages, nil
}
