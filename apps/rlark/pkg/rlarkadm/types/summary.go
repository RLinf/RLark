package types

import (
	"fmt"
	"strings"
)

// ComponentStatus is an exported type.
type ComponentStatus struct {
	Name    string
	Healthy bool
	Port    int32
	Address string
}

// InstallSummary summarizes results.
type InstallSummary struct {
	Plane               string
	Mode                string
	Namespace           string
	Components          []ComponentStatus
	ControlPlaneAddress string
	AdminPassword       string
	UserPassword        string
}

// Print is an exported method.
func (s *InstallSummary) Print() {
	var b strings.Builder

	b.WriteString("\n")
	_, _ = fmt.Fprintf(&b, "RLark %s plane installed successfully!\n", s.Plane)
	_, _ = fmt.Fprintf(&b, "Deployment mode: %s\n", s.Mode)
	if s.Namespace != "" {
		_, _ = fmt.Fprintf(&b, "Namespace: %s\n", s.Namespace)
	}
	b.WriteString("\n")
	b.WriteString("Components:\n")

	maxName := 0
	for _, c := range s.Components {
		if len(c.Name) > maxName {
			maxName = len(c.Name)
		}
	}

	for _, c := range s.Components {
		mark := "x"
		status := "Not Ready"
		if c.Healthy {
			mark = "v"
			status = "Running"
		}
		padding := strings.Repeat(" ", maxName-len(c.Name)+2)
		addr := ""
		if c.Address != "" {
			addr = " - " + c.Address
		}
		_, _ = fmt.Fprintf(&b, "  [%s] %s%s%s%s\n", mark, c.Name, padding, status, addr)
	}

	if s.Plane == "control" {
		b.WriteString("\n")
		b.WriteString("Frontend access:\n")
		_, _ = fmt.Fprintf(&b, "  kubectl port-forward -n %s svc/rlark-ui 8080:80\n", s.Namespace)
		b.WriteString("  Then open: http://localhost:8080\n")
		b.WriteString("\n")
		b.WriteString("Admin console:\n")
		b.WriteString("  http://localhost:8080/admin\n")
		if s.AdminPassword != "" {
			b.WriteString("\n")
			b.WriteString("Credentials:\n")
			b.WriteString("  Admin:  admin / " + s.AdminPassword + "\n")
			b.WriteString("  User:   user / " + s.UserPassword + "\n")
		}
	}

	if s.Plane == "data" && s.ControlPlaneAddress != "" {
		b.WriteString("\n")
		_, _ = fmt.Fprintf(&b, "Agent connecting to control plane: %s\n", s.ControlPlaneAddress)
	}

	b.WriteString("\n")
	fmt.Println(b.String())
}
