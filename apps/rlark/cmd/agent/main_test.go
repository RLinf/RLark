package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestLocalDiagnosticsDoNotConnect(t *testing.T) {
	for _, diagnostic := range []string{"check-config", "print-config"} {
		t.Run(diagnostic, func(t *testing.T) {
			t.Setenv("KUBECONFIG", "")
			cmd := newAgentCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{diagnostic, "--client-cert=/nonexistent/cert", "--client-key=secret-value", "--image=trusted/tools:v1", "--server-address=https://127.0.0.1:1"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "secret-value") {
				t.Fatal("diagnostic leaked private input")
			}
			if diagnostic == "print-config" && !strings.Contains(out.String(), `"sources"`) {
				t.Fatal("missing sources")
			}
			if diagnostic == "check-config" && out.Len() != 0 {
				t.Fatal("check-config must not print configuration")
			}
		})
	}
}

func TestCLIParsingErrorsAreRedacted(t *testing.T) {
	cmd := newAgentCommand()
	var out bytes.Buffer
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--kube-qps=secret-value"})
	err := cmd.Execute()
	if err == nil || strings.Contains(err.Error()+out.String(), "secret-value") {
		t.Fatal("invalid CLI leaked input or was accepted")
	}
}

func TestDiagnosticSubcommandsShareFlagsAndRejectLegacyFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--mode=node", "check-config", "--node-name=node-a", "--client-cert=cert", "--client-key=key"},
		{"print-config", "--mode=node", "--node-name=node-a", "--client-cert=cert", "--client-key=key"},
	} {
		cmd := newAgentCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), `"mode":"cluster"`) {
			t.Fatal("inherited override lost")
		}
	}
	for _, flag := range []string{"--check-config", "--print-config"} {
		cmd := newAgentCommand()
		var out bytes.Buffer
		cmd.SetErr(&out)
		cmd.SetArgs([]string{flag})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("legacy flag %s accepted", flag)
		}
	}
}
