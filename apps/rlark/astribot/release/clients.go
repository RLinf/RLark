package release

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// composeVariables is the variable of the control plane's env file that
// names each service's image (astribot/release/control-plane/rlark.compose.yaml).
var composeVariables = map[string]string{
	"rlark-server":             "RLARK_SERVER_IMAGE",
	"rlark-controller-manager": "RLARK_CONTROLLER_MANAGER_IMAGE",
	"rlark-gateway":            "RLARK_GATEWAY_IMAGE",
	"rlark-ui":                 "RLARK_UI_IMAGE",
	"astribot-iam":             "ASTRIBOT_IAM_IMAGE",
}

// Compose is the control plane as docker compose runs it on the management
// machine. Its env file always says what each service runs: a service is
// started anew only after its line says the new image.
type Compose struct {
	Project, File, Env string
}

func (c Compose) Images(ctx context.Context) (map[string]string, error) {
	return c.running(ctx, "{{.Config.Image}}")
}

func (c Compose) Configs(ctx context.Context) (map[string]string, error) {
	return c.running(ctx, `{{index .Config.Labels "com.docker.compose.config-hash"}}`)
}

// running is service -> what format reads of its container, where the container runs.
func (c Compose) running(ctx context.Context, format string) (map[string]string, error) {
	read := map[string]string{}
	for service := range composeVariables {
		out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Running}} "+format, service).CombinedOutput()
		if err != nil {
			if strings.Contains(strings.ToLower(string(out)), "no such object") {
				continue
			}
			return nil, fmt.Errorf("docker inspect %s: %w", service, err)
		}
		running, value, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
		if running == "true" {
			read[service] = value
		}
	}
	return read, nil
}

// env is the env file's variables, the services' images among them.
func (c Compose) env() (map[string]string, error) {
	data, err := os.ReadFile(c.Env)
	if err != nil {
		return nil, err
	}
	variables := map[string]string{}
	for line := range strings.Lines(string(data)) {
		if key, value, found := strings.Cut(strings.TrimSpace(line), "="); found && !strings.HasPrefix(key, "#") {
			variables[key] = value
		}
	}
	return variables, nil
}

func (c Compose) Compose(context.Context) (*ComposeFile, error) {
	file, err := os.ReadFile(c.File)
	if err != nil {
		return nil, err
	}
	variables, err := c.env()
	if err != nil {
		return nil, err
	}
	for _, image := range composeVariables {
		delete(variables, image)
	}
	return &ComposeFile{File: string(file), Variables: variables}, nil
}

// Hashes writes compose and images where the compose file is not, and reads every service's hash of them.
func (c Compose) Hashes(ctx context.Context, compose ComposeFile, images map[string]string) (map[string]string, error) {
	dir, err := os.MkdirTemp("", "rlark-compose-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	file, env := filepath.Join(dir, "compose.yaml"), filepath.Join(dir, "env")
	if err := os.WriteFile(file, []byte(compose.File), 0o600); err != nil {
		return nil, err
	}
	variables := maps.Clone(compose.Variables)
	for service, variable := range composeVariables {
		variables[variable] = images[service]
	}
	if err := os.WriteFile(env, envFile(variables), 0o600); err != nil {
		return nil, err
	}
	var stderr strings.Builder
	command := exec.CommandContext(ctx, "docker", "compose", "-p", c.Project, "-f", file, "--env-file", env, "config", "--hash", "*")
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose config --hash: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	hashes := map[string]string{}
	for line := range strings.Lines(string(out)) {
		if service, hash, found := strings.Cut(strings.TrimSpace(line), " "); found {
			hashes[service] = hash
		}
	}
	return hashes, nil
}

func (c Compose) Write(_ context.Context, compose ComposeFile) error {
	env, err := c.env()
	if err != nil {
		return err
	}
	variables := maps.Clone(compose.Variables)
	for _, image := range composeVariables {
		if value, set := env[image]; set {
			variables[image] = value
		}
	}
	if err := replaceFile(c.File, []byte(compose.File), 0o644); err != nil {
		return err
	}
	return replaceFile(c.Env, envFile(variables), 0o600)
}

// envFile is an env file of variables, one a line, sorted.
func envFile(variables map[string]string) []byte {
	var out strings.Builder
	for _, key := range slices.Sorted(maps.Keys(variables)) {
		fmt.Fprintf(&out, "%s=%s\n", key, variables[key])
	}
	return []byte(out.String())
}

// replaceFile writes a file whole: whoever reads it reads the old one or the new one.
func replaceFile(path string, data []byte, mode os.FileMode) error {
	next := path + ".next"
	if err := os.WriteFile(next, data, mode); err != nil {
		return err
	}
	return os.Rename(next, path)
}

func (c Compose) Pull(ctx context.Context, image string) error {
	if out, err := exec.CommandContext(ctx, "docker", "pull", "--quiet", image).CombinedOutput(); err != nil {
		return fmt.Errorf("docker pull %s: %w: %s", image, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c Compose) Set(ctx context.Context, service, image string) error {
	variables, err := c.env()
	if err != nil {
		return err
	}
	compose := []string{"compose", "-p", c.Project, "-f", c.File, "--env-file", c.Env}
	services, err := exec.CommandContext(ctx, "docker", append(compose, "config", "--services")...).Output()
	if err != nil {
		return fmt.Errorf("docker compose config --services: %w", err)
	}
	if !slices.Contains(strings.Fields(string(services)), service) {
		return c.removeOrphan(ctx, service)
	}
	variables[composeVariables[service]] = image
	if err := replaceFile(c.Env, envFile(variables), 0o600); err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, "docker", append(compose, "up", "-d", "--quiet-pull", "--no-deps", service)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up %s: %w: %s", service, err, out)
	}
	// A service that depends on this one holds the address of the container it replaced (the console's nginx
	// resolved rlark-gateway when it started, and answered 502 after the gateway alone was replaced, 11-19):
	// compose does not restart it for an up of the dependency alone, so it is restarted here.
	out, err = exec.CommandContext(ctx, "docker", append(compose, "config", "--format", "json")...).Output()
	if err != nil {
		return fmt.Errorf("docker compose config: %w", err)
	}
	var config struct {
		Services map[string]struct {
			DependsOn map[string]any `json:"depends_on"`
		} `json:"services"`
	}
	if err := json.Unmarshal(out, &config); err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(config.Services)) {
		if _, depends := config.Services[name].DependsOn[service]; !depends {
			continue
		}
		if out, err := exec.CommandContext(ctx, "docker", append(compose, "restart", name)...).CombinedOutput(); err != nil {
			return fmt.Errorf("docker compose restart %s: %w: %s", name, err, out)
		}
	}
	return nil
}

func (c Compose) removeOrphan(ctx context.Context, service string) error {
	format := `{{index .Config.Labels "com.docker.compose.project"}} {{index .Config.Labels "com.docker.compose.service"}}`
	out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", format, service).CombinedOutput()
	if err != nil {
		if strings.Contains(strings.ToLower(string(out)), "no such object") {
			return nil
		}
		return fmt.Errorf("docker inspect orphan %s: %w", service, err)
	}
	if got, want := strings.TrimSpace(string(out)), c.Project+" "+service; got != want {
		return fmt.Errorf("refuse to remove container %s: compose ownership is %q, want %q", service, got, want)
	}
	if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", service).CombinedOutput(); err != nil {
		return fmt.Errorf("docker rm orphan %s: %w: %s", service, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// logTo writes every line to out with the time of day, and to the record.
type logTo struct {
	out    io.Writer
	record *bufio.Writer
}

func (l logTo) printf(format string, args ...any) {
	line := time.Now().Format("15:04:05") + " " + fmt.Sprintf(format, args...) + "\n"
	_, _ = io.WriteString(l.out, line)
	if l.record != nil {
		_, _ = l.record.WriteString(line)
		_ = l.record.Flush()
	}
}
