package gateway

import (
	"context"
	"fmt"
	"io"
	"strings"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/nodessh"
	"golang.org/x/crypto/ssh"
)

// provisionSSH is a login to a new cluster's first machine with the account an administrator gave: scripts run as
// root with sudo, which reads the password on its standard input first.
type provisionSSH struct {
	client   *ssh.Client
	user     string
	password []byte
}

// dialProvisionMachine logs in and checks the account may use sudo.
func dialProvisionMachine(ctx context.Context, machine rlarkv1alpha1.ClusterProvisionMachine, password []byte, hostKey string) (provisionMachine, string, error) {
	client, shown, dialErr := nodessh.Dial(ctx, machine.Address, int(machine.Port), machine.User, hostKey, password)
	if dialErr != nil {
		return nil, "", dialErr
	}
	if sudoErr := nodessh.CheckSudo(client, machine.User, password); sudoErr != nil {
		_ = client.Close()
		return nil, "", sudoErr
	}
	return &provisionSSH{client: client, user: machine.User, password: password}, shown, nil
}

func (m *provisionSSH) Run(_ context.Context, script string, stdin io.Reader, line func(string)) (string, error) {
	input := nodessh.SudoInput(m.password)
	defer clear(input)
	var out strings.Builder
	err := nodessh.Follow(m.client, nodessh.Sudo(m.user)+"sh -c "+nodessh.Quote(script)+" 2>&1", nodessh.SudoStdin(m.user, input, stdin), func(text string) {
		out.WriteString(text + "\n")
		if line != nil {
			line(text)
		}
	})
	if err != nil {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		return out.String(), fmt.Errorf("%v: %s", err, strings.Join(lines[max(0, len(lines)-5):], " | "))
	}
	return out.String(), nil
}

func (m *provisionSSH) Close() error { return m.client.Close() }
