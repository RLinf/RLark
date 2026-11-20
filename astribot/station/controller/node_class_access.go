package controller

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// classHostKeyAlgorithms is the one type of host key a machine of a class is
// asked for: the fingerprints a class declares, a scan reports and a Machine
// records are all of the host's ED25519 key.
var classHostKeyAlgorithms = []string{ssh.KeyAlgoED25519}

// NodeClassAccess logs in to the machines of a NodeClass with the accounts
// every machine of the class has. Their passwords are values of secrets kept
// in RLark, read from the secret's declaration on the management plane at
// every login: nothing of them is copied, written to a status or given to
// another host than one that has shown the key it is known by.
type NodeClassAccess struct {
	Reader client.Reader
	// Namespace is where RLark keeps the declarations of its secrets.
	Namespace string
}

// ErrClassAccessUnconfigured says a password of a class has not been entered.
var ErrClassAccessUnconfigured = errors.New("a password of the NodeClass has not been entered in RLark")

// classAccounts are the passwords of a class: the entry device's account,
// the node's account, and the node's sudo.
type classAccounts struct{ entry, node, sudo []byte }

func (a classAccounts) clear() {
	clear(a.entry)
	clear(a.node)
	clear(a.sudo)
}

// secretValue reads one value of a secret kept in RLark. The declaration is a
// Secret named js-<name>-replication whose manifests hold the delivery, whose
// manifests hold the Secret every selected cluster gets (apps/rlark/pkg/gateway,
// job secrets).
func (s NodeClassAccess) secretValue(ctx context.Context, value stationv1alpha1.SecretValue) ([]byte, error) {
	declaration := &corev1.Secret{}
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: "js-" + value.Secret + "-replication"}, declaration); apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("%w: secret %s", ErrClassAccessUnconfigured, value.Secret)
	} else if err != nil {
		return nil, fmt.Errorf("read secret %s: %w", value.Secret, err)
	}
	var deliveries, delivered []corev1.Secret
	if err := yaml.Unmarshal(declaration.Data["manifests"], &deliveries); err != nil || len(deliveries) != 1 {
		return nil, fmt.Errorf("secret %s: its declaration is not one RLark wrote", value.Secret)
	}
	if err := yaml.Unmarshal(deliveries[0].Data["manifests"], &delivered); err != nil || len(delivered) != 1 {
		return nil, fmt.Errorf("secret %s: its declaration is not one RLark wrote", value.Secret)
	}
	found := delivered[0].StringData[value.Key]
	if found == "" {
		return nil, fmt.Errorf("%w: secret %s has no %s", ErrClassAccessUnconfigured, value.Secret, value.Key)
	}
	return []byte(found), nil
}

func (s NodeClassAccess) class(ctx context.Context, name string) (*stationv1alpha1.NodeClass, classAccounts, error) {
	class := &stationv1alpha1.NodeClass{}
	if err := s.Reader.Get(ctx, types.NamespacedName{Name: name}, class); err != nil {
		return nil, classAccounts{}, fmt.Errorf("read NodeClass %s: %w", name, err)
	}
	if class.Spec.Access == nil {
		return nil, classAccounts{}, fmt.Errorf("NodeClass %s declares no access", name)
	}
	access := class.Spec.Access
	sudoValue := access.Node.Password
	if access.Node.SudoPassword != nil {
		sudoValue = *access.Node.SudoPassword
	}
	var accounts classAccounts
	var err error
	if accounts.entry, err = s.secretValue(ctx, access.Entry.Password); err == nil {
		if accounts.node, err = s.secretValue(ctx, access.Node.Password); err == nil {
			accounts.sudo, err = s.secretValue(ctx, sudoValue)
		}
	}
	if err != nil {
		accounts.clear()
		return nil, classAccounts{}, fmt.Errorf("the accounts of NodeClass %s: %w", name, err)
	}
	return class, accounts, nil
}

// MachineAddress is where and by which keys one machine of a class is dialed.
type MachineAddress struct {
	// Entry is the entry device's address on its site's network.
	Entry, EntryHostKey string
	// Via is the relay of the hub of the machine's site.
	Via string
	// NodeHostKey is empty for a machine nobody has logged in to yet; the
	// key it shows through its verified entry device is then returned.
	NodeHostKey string
}

func entryConfig(class *stationv1alpha1.NodeClass, accounts classAccounts, at MachineAddress) NodeSSHConfig {
	entry := class.Spec.Access.Entry
	return NodeSSHConfig{Address: net.JoinHostPort(at.Entry, strconv.Itoa(int(entry.Port))), User: entry.User, HostKeyFingerprint: at.EntryHostKey,
		Password: accounts.entry, HostKeyAlgorithms: classHostKeyAlgorithms, Via: at.Via}
}

// dialEntry logs in to the entry device of a machine. The returned sudo
// password is nil where the account's sudo asks for none.
func dialEntry(ctx context.Context, class *stationv1alpha1.NodeClass, accounts classAccounts, at MachineAddress) (*ssh.Client, []byte, error) {
	client, err := DialNodeSSH(ctx, entryConfig(class, accounts, at))
	if err != nil {
		return nil, nil, err
	}
	return client, sudoPassword(ctx, client, accounts.entry), nil
}

// dialNode logs in to the computer behind the entry device.
func dialNode(ctx context.Context, class *stationv1alpha1.NodeClass, accounts classAccounts, at MachineAddress) (client *ssh.Client, sudo []byte, hostKey string, err error) {
	jump := entryConfig(class, accounts, at)
	node := class.Spec.Access.Node
	hostKey = at.NodeHostKey
	client, err = DialNodeSSH(ctx, NodeSSHConfig{Address: net.JoinHostPort(node.Address, strconv.Itoa(int(node.Port))), User: node.User, HostKeyFingerprint: at.NodeHostKey,
		Password: accounts.node, HostKeyAlgorithms: classHostKeyAlgorithms, Jump: &jump, LearnedHostKey: &hostKey})
	if err != nil {
		return nil, nil, "", err
	}
	return client, sudoPassword(ctx, client, accounts.sudo), hostKey, nil
}

// sudoPassword is nil for an account whose sudo asks for no password: sudo
// would leave the password on stdin, as the first line of the script.
func sudoPassword(ctx context.Context, client *ssh.Client, password []byte) []byte {
	if _, err := RunNodeSSHStream(ctx, client, "sudo -n true", nil); err == nil {
		return nil
	}
	return password
}

// machineProblem names what a failed login says of the machine, or that its
// site's hub does not answer; empty for any other fault.
func machineProblem(err error) string {
	var mismatch *SSHHostKeyMismatchError
	behind := errors.Is(err, ErrSSHBehindJump)
	switch {
	case errors.Is(err, ErrClassAccessUnconfigured):
		return stationv1alpha1.MachineAccessUnconfigured
	case behind && errors.Is(err, ErrSSHCredentialRejected):
		return stationv1alpha1.MachineNodeCredentialRejected
	case behind && errors.As(err, &mismatch):
		return stationv1alpha1.MachineNodeHostKeyChanged
	case errors.Is(err, ErrSSHCredentialRejected):
		return stationv1alpha1.MachineCredentialRejected
	case errors.As(err, &mismatch):
		return stationv1alpha1.MachineHostKeyChanged
	case errors.Is(err, ErrSSHJumpUnreachable), errors.Is(err, ErrSSHSiteHostUnreachable):
		return stationv1alpha1.MachineEntryUnreachable
	case errors.Is(err, ErrSSHRelayUnreachable):
		return stationv1alpha1.MachineHubUnreachable
	case errors.Is(err, ErrSSHTargetUnreachable):
		return stationv1alpha1.MachineNodeUnreachable
	}
	return ""
}
