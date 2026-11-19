package roscontroller

import (
	"os"
	"path/filepath"
	"testing"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
)

// TestParseLaunchArgs verifies extraction of user-configurable <arg> tags from
// a ROS launch file: args with defaults are optional, args without defaults
// are required, and value-assigned args are skipped.
func TestParseLaunchArgs(t *testing.T) {
	content := `<launch>
  <arg name="robot_ip" doc="Robot IP address" />
  <arg name="rate" default="100" doc="Control rate (Hz)" />
  <arg name="mode" value="position" />
  <arg name="verbose" default="false" documentation="Verbose logging" />
  <arg name="port" value="8080" default="8080" />
</launch>`

	args := parseLaunchArgs(content)

	byName := make(map[string]*pb.LaunchArg, len(args))
	for _, a := range args {
		byName[a.Name] = a
	}

	// robot_ip: no default → required, doc from "doc" attr.
	rip, ok := byName["robot_ip"]
	if !ok {
		t.Fatal("robot_ip arg missing")
	}
	if !rip.Required {
		t.Error("robot_ip should be required (no default)")
	}
	if rip.Default != "" {
		t.Errorf("robot_ip default = %q, want empty", rip.Default)
	}
	if rip.Description != "Robot IP address" {
		t.Errorf("robot_ip doc = %q", rip.Description)
	}

	// rate: has default → not required.
	rate, ok := byName["rate"]
	if !ok {
		t.Fatal("rate arg missing")
	}
	if rate.Required {
		t.Error("rate should not be required (has default)")
	}
	if rate.Default != "100" {
		t.Errorf("rate default = %q, want 100", rate.Default)
	}
	if rate.Description != "Control rate (Hz)" {
		t.Errorf("rate doc = %q", rate.Description)
	}

	// mode: value-assigned, no default → skipped.
	if _, ok := byName["mode"]; ok {
		t.Error("value-assigned arg (mode) should be skipped")
	}

	// verbose: documentation attr fallback.
	verb, ok := byName["verbose"]
	if !ok {
		t.Fatal("verbose arg missing")
	}
	if verb.Default != "false" {
		t.Errorf("verbose default = %q", verb.Default)
	}
	if verb.Description != "Verbose logging" {
		t.Errorf("verbose doc = %q (should fall back to documentation attr)", verb.Description)
	}

	// port: has both value and default → NOT skipped (default present).
	port, ok := byName["port"]
	if !ok {
		t.Fatal("port arg missing (should be kept when default is present)")
	}
	if port.Default != "8080" {
		t.Errorf("port default = %q", port.Default)
	}
}

// TestParseLaunchArgs_Empty verifies that an empty or arg-less launch file
// returns no args.
func TestParseLaunchArgs_Empty(t *testing.T) {
	if got := parseLaunchArgs(""); len(got) != 0 {
		t.Errorf("empty input = %v, want none", got)
	}
	if got := parseLaunchArgs(`<launch></launch>`); len(got) != 0 {
		t.Errorf("no-arg launch = %v, want none", got)
	}
}

// TestParseLaunchArgs_MalformedXML verifies that malformed XML does not panic
// and returns whatever args were parsed before the error.
func TestParseLaunchArgs_MalformedXML(t *testing.T) {
	content := `<launch><arg name="ok" default="1" /><broken`
	args := parseLaunchArgs(content)
	if len(args) != 1 || args[0].Name != "ok" {
		t.Errorf("got %v, want 1 arg named ok", args)
	}
}

// TestParsePackageInfo verifies reading and XML-parsing of a ROS package.xml.
func TestParsePackageInfo(t *testing.T) {
	dir := t.TempDir()
	xmlContent := `<?xml version="1.0"?>
<package format="2">
  <name>franka_control</name>
  <version>0.8.1</version>
  <description>Franka Emika robot controller</description>
  <maintainer email="me@example.com">Maint Name</maintainer>
</package>`
	if err := os.WriteFile(filepath.Join(dir, "package.xml"), []byte(xmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	pkg, err := parsePackageInfo(dir)
	if err != nil {
		t.Fatalf("parsePackageInfo: %v", err)
	}
	if pkg.Name != "franka_control" || pkg.Version != "0.8.1" ||
		pkg.Description != "Franka Emika robot controller" || pkg.Maintainer != "Maint Name" {
		t.Errorf("package = %+v", pkg)
	}
}

// TestParsePackageInfo_MissingFile verifies that a missing package.xml returns
// an error.
func TestParsePackageInfo_MissingFile(t *testing.T) {
	_, err := parsePackageInfo(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing package.xml")
	}
}

// TestParsePackageInfo_InvalidXML verifies that malformed XML returns an error.
func TestParsePackageInfo_InvalidXML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.xml"), []byte("<package><name>"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := parsePackageInfo(dir)
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

// TestIsPackageAllowed verifies the whitelist logic: nil = none, "*" = all,
// otherwise exact match.
func TestIsPackageAllowed(t *testing.T) {
	t.Run("nil_whitelist", func(t *testing.T) {
		c := NewController("10.0.0.1", nil)
		if c.isPackageAllowed("any") {
			t.Error("nil whitelist should disallow all")
		}
	})
	t.Run("empty_whitelist", func(t *testing.T) {
		c := NewController("10.0.0.1", []string{})
		if c.isPackageAllowed("any") {
			t.Error("empty whitelist should disallow all")
		}
	})
	t.Run("star_allows_all", func(t *testing.T) {
		c := NewController("10.0.0.1", []string{"*"})
		if !c.isPackageAllowed("any_pkg") {
			t.Error("\"*\" should allow all packages")
		}
	})
	t.Run("exact_match", func(t *testing.T) {
		c := NewController("10.0.0.1", []string{"pkg_a", "pkg_b"})
		if !c.isPackageAllowed("pkg_a") {
			t.Error("pkg_a should be allowed")
		}
		if c.isPackageAllowed("pkg_c") {
			t.Error("pkg_c should not be allowed")
		}
	})
}

// TestModeCfgDisplayName verifies the "pkg/launch_file" display name for
// custom (ad-hoc) modes.
func TestModeCfgDisplayName(t *testing.T) {
	if got := modeCfgDisplayName("my_pkg", "custom.launch"); got != "my_pkg/custom.launch" {
		t.Errorf("got %q, want my_pkg/custom.launch", got)
	}
}

// TestModeConfigToInfo verifies the protobuf conversion, including the nil
// case for empty configs.
func TestModeConfigToInfo(t *testing.T) {
	t.Run("empty_returns_nil", func(t *testing.T) {
		if info := modeConfigToInfo("x", ModeConfig{}); info != nil {
			t.Errorf("empty config should return nil, got %+v", info)
		}
	})
	t.Run("populated", func(t *testing.T) {
		mc := ModeConfig{
			Package:    "pkg",
			LaunchFile: "f.launch",
			Args:       map[string]string{"a": "1"},
			Env:        map[string]string{"E": "v"},
			ArgFrom:    map[string]string{"arg": "param"},
		}
		info := modeConfigToInfo("joint", mc)
		if info == nil {
			t.Fatal("expected non-nil")
		}
		if info.Name != "joint" || info.Package != "pkg" || info.LaunchFile != "f.launch" {
			t.Errorf("info = %+v", info)
		}
		if info.Args["a"] != "1" || info.Env["E"] != "v" || info.ArgFrom["arg"] != "param" {
			t.Errorf("maps not copied: %+v", info)
		}
	})
}

// TestModeConfigFromProto verifies the proto-to-struct conversion preserves all
// fields.
func TestModeConfigFromProto(t *testing.T) {
	pbCfg := &pb.ModeConfig{
		Package:              "pkg",
		LaunchFile:           "f.launch",
		Args:                 map[string]string{"a": "1"},
		PassthroughRobotArgs: true,
		ArgFrom:              map[string]string{"arg": "param"},
		Env:                  map[string]string{"E": "v"},
	}
	mc := modeConfigFromProto(pbCfg)
	if mc.Package != "pkg" || mc.LaunchFile != "f.launch" {
		t.Errorf("pkg/launch: %+v", mc)
	}
	if !mc.PassthroughRobotArgs || mc.Args["a"] != "1" || mc.ArgFrom["arg"] != "param" || mc.Env["E"] != "v" {
		t.Errorf("fields not copied: %+v", mc)
	}
}
