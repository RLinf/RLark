package utils

import "fmt"

// ChildName returns a child name derived from parent and child.
func ChildName(parent, child string) string {
	return fmt.Sprintf("%s-%s", parent, child)
}
