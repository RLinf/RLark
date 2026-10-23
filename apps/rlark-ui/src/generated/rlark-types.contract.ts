import type { INode, ITask } from "./rlark-types";

// These assignments are compile-time checks for Kubernetes-native structures
// embedded in RLark CRDs. Keep them free of runtime imports.
export const generatedNodeContract: INode = {
  apiVersion: "rlinf.io/v1alpha1",
  kind: "Node",
  status: {
    capacity: {
      cpu: "4",
      memory: "16Gi",
    },
  },
};

export const generatedTaskContract: ITask = {
  apiVersion: "rlinf.io/v1alpha1",
  kind: "Task",
  spec: {
    role: "Worker",
    placement: {},
    runtime: { kind: "Kubernetes" },
    workload: {
      kind: "rlinf.io/ray",
      version: "v1",
      carrier: { type: "rlinf.io/kubernetes-deployment", version: "v1" },
      template: {
        spec: {
          containers: [
            {
              name: "main",
              image: "example.invalid/worker:latest",
              ports: [{ containerPort: 8080, hostPort: 8080 }],
              resources: {
                requests: {
                  cpu: "500m",
                  memory: "1Gi",
                },
              },
            },
          ],
        },
      },
    },
  },
};
