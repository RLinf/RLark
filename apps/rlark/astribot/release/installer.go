package release

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// The installer is the one thing of Station that is not an image of a Pod: Station API serves it to the machines that
// join and its controller sends it to the ones it joins over SSH, both from a file on Station API's machine. The site
// registry's CA is in it, so it is an environment's (installer), not a release's: one artifact
// (rlark/staging-artifact, <tag>.bundle.tar at its root, astribot/station/bootstrap/Dockerfile.installer) per site.
const (
	// installerAnnotation is, on both Deployments, the artifact the file they read came from.
	installerAnnotation = "rlark.io/installer"
	// The flags that name the file, as each Deployment is started with them, and where the file is in their Pods.
	apiBundleFlag        = "--bootstrap-bundle-file="
	controllerBundleFlag = "--installer-bundle="
	bootstrapMount       = "/var/run/station-bootstrap/"
	// BootstrapUser reads the file: both run as it.
	BootstrapUser = 65532
)

// InstallerHost is Station API's machine, which the command runs on.
type InstallerHost interface {
	// Put lays down the installer the artifact holds beside the earlier ones, readable by Station's containers, and
	// says the file's name and its SHA-256.
	Put(ctx context.Context, artifact string) (file, sum string, err error)
	// Served is the SHA-256 of the installer Station API serves now.
	Served(ctx context.Context) (string, error)
}

// StationBootstrap is the InstallerHost of a site: Dir is the directory both Deployments mount (bootstrapMount), URL
// Station API as a machine that joins reaches it, the CA it is verified with Dir/ca.crt; Owner the user the files are
// given to (BootstrapUser).
type StationBootstrap struct {
	Dir, URL string
	Owner    int
	Registry *RegistryClient
}

func (b *StationBootstrap) Put(ctx context.Context, artifact string) (string, string, error) {
	repository, digest, _ := strings.Cut(strings.TrimPrefix(artifact, b.Registry.Registry+"/"), "@")
	data, _, err := b.Registry.manifest(ctx, repository, digest)
	if err != nil {
		return "", "", err
	}
	var manifest struct {
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", "", fmt.Errorf("installer %s: %w", artifact, err)
	}
	for _, layer := range manifest.Layers {
		file, sum, err := b.unpack(ctx, repository, layer.Digest)
		if err != nil || file != "" {
			return file, sum, err
		}
	}
	return "", "", fmt.Errorf("installer %s holds no .bundle.tar", artifact)
}

// unpack writes the .bundle.tar a layer (gzip-compressed, or a tar as it is) holds, if it holds one, under its own
// name; a file of that name that is there already is left as it is (an installer's name is its commit's).
func (b *StationBootstrap) unpack(ctx context.Context, repository, digest string) (string, string, error) {
	body, err := b.Registry.blob(ctx, repository, digest)
	if err != nil {
		return "", "", err
	}
	defer body.Close()
	var layer io.Reader = bufio.NewReader(body)
	if magic, _ := layer.(*bufio.Reader).Peek(2); bytes.Equal(magic, []byte{0x1f, 0x8b}) {
		if layer, err = gzip.NewReader(layer); err != nil {
			return "", "", err
		}
	}
	archive := tar.NewReader(layer)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return "", "", nil
		}
		if err != nil {
			return "", "", err
		}
		name := filepath.Base(header.Name)
		if header.Typeflag != tar.TypeReg || !strings.HasSuffix(name, ".bundle.tar") {
			continue
		}
		target := filepath.Join(b.Dir, name)
		if sum, err := fileSum(target); err == nil {
			return name, sum, nil
		}
		temporary, err := os.CreateTemp(b.Dir, "."+name+".*")
		if err != nil {
			return "", "", err
		}
		defer os.Remove(temporary.Name())
		hash := sha256.New()
		_, err = io.Copy(io.MultiWriter(temporary, hash), archive)
		if closed := temporary.Close(); err == nil {
			err = closed
		}
		if err == nil {
			err = os.Chmod(temporary.Name(), 0o644)
		}
		if err == nil {
			err = os.Chown(temporary.Name(), b.Owner, -1)
		}
		if err == nil {
			err = os.Rename(temporary.Name(), target)
		}
		return name, hex.EncodeToString(hash.Sum(nil)), err
	}
}

func (b *StationBootstrap) Served(ctx context.Context) (string, error) {
	ca, err := os.ReadFile(filepath.Join(b.Dir, "ca.crt"))
	if err != nil {
		return "", err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	client := &http.Client{Timeout: 10 * time.Minute, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(b.URL, "/")+"/v1/bootstrap/bundle", nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Station API serves the installer with %s", response.Status)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, response.Body); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fileSum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// pointAtInstaller makes a Station Deployment read the file (its flag) and says which artifact it is.
func pointAtInstaller(deployment *appsv1.Deployment, flag, file, artifact string) error {
	container := &deployment.Spec.Template.Spec.Containers[0]
	for i, arg := range container.Args {
		if strings.HasPrefix(arg, flag) {
			container.Args[i] = flag + bootstrapMount + file
			if deployment.Annotations == nil {
				deployment.Annotations = map[string]string{}
			}
			deployment.Annotations[installerAnnotation] = artifact
			return nil
		}
	}
	return fmt.Errorf("%s/%s is started without %s", deployment.Namespace, deployment.Name, flag)
}

// installerSettled: both Deployments read the file and say the artifact, both are rolled out, and what Station API
// serves is that file.
func installerSettled(ctx context.Context, member kubernetes.Interface, host InstallerHost, artifact, file, sum string) (string, error) {
	for name, flag := range map[string]string{stationAPI: apiBundleFlag, stationController: controllerBundleFlag} {
		deployment, err := member.AppsV1().Deployments(stationNamespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if deployment.Annotations[installerAnnotation] != artifact || !slices.Contains(deployment.Spec.Template.Spec.Containers[0].Args, flag+bootstrapMount+file) {
			return fmt.Sprintf("%s/%s does not read the installer %s", stationNamespace, name, file), nil
		}
		if reading, err := deploymentSettled(ctx, member, stationNamespace, name, deployment.Spec.Template.Spec.Containers[0].Image); err != nil || reading != "" {
			return reading, err
		}
	}
	served, err := host.Served(ctx)
	if err != nil {
		return "Station API: " + err.Error(), nil
	}
	if served != sum {
		return fmt.Sprintf("Station API serves an installer of SHA-256 %s, not %s's %s", served[:12], file, sum[:12]), nil
	}
	return "", nil
}
