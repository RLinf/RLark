import type { DeploymentConfig } from "../backend.js";
import type { SignAgentCertResponse } from "../types.js";

export const defaultDeploymentConfig: DeploymentConfig = {
  controlPlaneAddress: "",
  sshAddress: "",
  insecureSkipTlsVerify: false,
  kubernetes: {
    kubeconfig: "~/.kube/config",
    agentImage: "rlark:latest",
    image: "rlark:latest",
    imagePullPolicy: "Always",
    imagePullSecrets: [],
    containerdSocket: "/run/containerd/containerd.sock",
  },
};

export function resolveDeploymentConfig(
  config: DeploymentConfig = {},
): DeploymentConfig {
  return {
    ...defaultDeploymentConfig,
    ...config,
    kubernetes: {
      ...defaultDeploymentConfig.kubernetes,
      ...config.kubernetes,
    },
  };
}

const indentPEM = (value: string) =>
  value
    .split("\n")
    .map((line) => `    ${line}`)
    .join("\n");

export function buildDeployYaml(
  result: SignAgentCertResponse,
  config: DeploymentConfig = {},
) {
  const resolved = resolveDeploymentConfig(config);
  const kubernetes = [
    resolved.kubernetes?.kubeconfig
      ? `  kubeconfig: ${resolved.kubernetes.kubeconfig}`
      : "",
    `  agent-image: ${resolved.kubernetes?.agentImage}`,
    resolved.kubernetes?.image ? `  image: ${resolved.kubernetes.image}` : "",
    resolved.kubernetes?.imagePullPolicy
      ? `  image-pull-policy: ${resolved.kubernetes.imagePullPolicy}`
      : "",
    resolved.kubernetes?.imagePullSecrets?.length
      ? `  image-pull-secrets: [${resolved.kubernetes.imagePullSecrets.join(", ")}]`
      : "",
    resolved.kubernetes?.containerdSocket
      ? `  containerd-socket: ${resolved.kubernetes.containerdSocket}`
      : "",
  ].filter(Boolean);

  return `apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: data
control-plane-address: ${resolved.controlPlaneAddress || result.server_addr}
${resolved.sshAddress ? `ssh-address: ${resolved.sshAddress}\n` : ""}${resolved.insecureSkipTlsVerify ? "insecure-skip-tls-verify: true\n" : ""}
cert:
  ca-cert: |
${indentPEM(result.ca_cert)}
  agent-cert: |
${indentPEM(result.agent_cert)}
  agent-key: |
${indentPEM(result.agent_key)}

kubernetes:
${kubernetes.join("\n")}
`;
}
