package constants

// Constants used by the package.
const (
	Namespace = "rlark-system"

	ComponentGateway           = "rlark-gateway"
	ComponentControllerManager = "rlark-controller-manager"
	ComponentServer            = "rlark-server"
	ComponentAgent             = "rlark-agent"
	ComponentAgentNode         = "rlark-agent-node"
	ComponentPostgresql        = "postgresql"
	ComponentKCP               = "kcp"
	ComponentEtcd              = "etcd"
	ComponentUI                = "rlark-ui"
)

// Constants used by the package.
const (
	EtcdDataDir      = "/var/run/etcd/default.etcd"
	EtcdClientPort   = 2379
	EtcdPeerPort     = 2380
	EtcdMetricsPort  = 8080
	EtcdClusterToken = "rlark-etcd-cluster"
)

// Constants used by the package.
const (
	ServerSSHPort = 2222
)

// Constants used by the package.
const (
	DBConfigPath      = "/etc/rlark/db.yaml"
	CertDir           = "/etc/rlark/certs"
	KCPDataDir        = "/.kcp"
	KCPEtcdDataDir    = "/.kcp/etcd-server"
	KCPKubeconfigPath = "/etc/kcp/admin.kubeconfig"
	PostgresqlDataDir = "/var/lib/postgresql/data"
	PostgresqlInitDir = "/docker-entrypoint-initdb.d"
)

// InitDBSQL is a constant value.
const InitDBSQL = `-- scripts/init-db.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
GRANT ALL PRIVILEGES ON DATABASE rlark TO postgres;
`

// Constants used by the package.
const (
	PrometheusScrapeLabelKey = "prometheus.io/scrape"
	PrometheusScrapeLabelVal = "true"
	PrometheusPortLabelKey   = "prometheus.io/port"
)

// Constants used by the package.
const (
	UIAuthSecretName = "rlark-ui-auth"
)
