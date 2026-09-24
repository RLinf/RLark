import assert from "node:assert/strict";
import test from "node:test";

import { buildDeployYaml } from "../dist/test/utils/deployYaml.js";

const certificate = {
  cluster_id: "cluster-a",
  server_addr: "https://signed.example.com:8443",
  ca_cert: "CA-LINE-1\nCA-LINE-2",
  agent_cert: "CERT",
  agent_key: "KEY",
};

test("deploy YAML uses system deployment defaults", () => {
  const yaml = buildDeployYaml(certificate, {
    controlPlaneAddress: "https://configured.example.com:8443",
    sshAddress: "client@configured.example.com:2222",
    kubernetes: {
      kubeconfig: "/etc/kubernetes/admin.conf",
      agentImage: "registry.example.com/rlark-agent:v1",
      image: "registry.example.com/rlark:v1",
      imagePullPolicy: "IfNotPresent",
      imagePullSecrets: ["registry-secret"],
      containerdSocket: "/run/k3s/containerd/containerd.sock",
    },
  });

  assert.match(
    yaml,
    /control-plane-address: https:\/\/configured\.example\.com:8443/,
  );
  assert.match(yaml, /kubeconfig: \/etc\/kubernetes\/admin\.conf/);
  assert.match(yaml, /agent-image: registry\.example\.com\/rlark-agent:v1/);
  assert.match(yaml, /image: registry\.example\.com\/rlark:v1/);
  assert.match(yaml, /image-pull-policy: IfNotPresent/);
  assert.match(yaml, /image-pull-secrets: \[registry-secret\]/);
  assert.match(
    yaml,
    /containerd-socket: \/run\/k3s\/containerd\/containerd\.sock/,
  );
  assert.match(yaml, /    CA-LINE-1\n    CA-LINE-2/);
});

test("deploy YAML keeps existing defaults when deployment config is absent", () => {
  const yaml = buildDeployYaml(certificate);
  assert.match(
    yaml,
    /control-plane-address: https:\/\/signed\.example\.com:8443/,
  );
  assert.match(yaml, /kubeconfig: ~\/\.kube\/config/);
  assert.match(yaml, /agent-image: rlark:latest/);
  assert.match(yaml, /image: rlark:latest/);
  assert.match(yaml, /image-pull-policy: Always/);
  assert.match(yaml, /containerd-socket: \/run\/containerd\/containerd\.sock/);
});
