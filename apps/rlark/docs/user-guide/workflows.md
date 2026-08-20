# Workflows

Use a Workflow to connect Job templates as a DAG. Each template becomes a child Job after its dependencies succeed.

## Prerequisites

Before creating a Workflow:

- Confirm the control plane and Workflow controller are running and that you can create standalone Jobs.
- Onboard every target cluster and verify that its nodes appear as available Workers.
- Make each referenced image accessible from its target cluster, and create any required Domain, storage class, or PVC first.
- Prepare each stage so that it exits with status 0 only after its work is complete; dependent stages are released from the child Job status, not from shell output.

## Task 1: Create the DAG

1. Open **Workflows** and choose **Create Workflow**.
2. Enter the Workflow name.
3. In **DAG Editor**, add a Job node for each stage.
4. Double-click a node name to rename it.
5. Drag from a node's right output port to the target node to create a dependency. Click an edge to remove it.
6. Confirm that the graph has no self-loop or cycle; the editor rejects both.

## Task 2: Configure Each Job

1. Continue to **Job Details**.
2. Select each Job tab in turn.
3. Configure its type, roles, Header role, target cluster, Worker resources, node selector, image, environment, storage, Domain, and run script as required.
4. Ensure every role has a target cluster and image and matches at least one available Worker.

The Job options follow the standalone Job form, but SSH key and TensorBoard settings are not included in the current Workflow form.

## Task 3: Review and Submit

1. Continue to **YAML Preview**.
2. Confirm that Workflow and template names are unique and valid Kubernetes resource names.
3. Verify each template's `dependencies`, image, storage, Domain, and task configuration.
4. Choose **Create Workflow**.

The console submits a Workflow CR. It does not generate a shell installation command.

## Task 4: Monitor the Run

1. Open the Workflow from the list.
2. Review the DAG execution view and the child Job table.
3. Select a non-pending DAG node to open its generated child Job. Generated Job names use `<workflow-name>-<template-name>`.
4. Inspect each child Job's Workers and logs when troubleshooting.

A dependent stage starts only after its predecessors succeed. If a predecessor's script has ended but its Job remains Running, check whether background processes are still active.

## Validate the Result

- Confirm the Workflow phase becomes `Succeeded` and every DAG node is `Succeeded`.
- Confirm the child Job table contains one Job for every template and that each generated name matches `<workflow-name>-<template-name>`.
- Open each child Job and verify its Worker count, target cluster, logs, and expected output or artifacts. A green DAG alone does not validate application-level results.

## Handle Failures

If any child Job becomes `Failed`, the Workflow becomes terminal `Failed`, and dependent stages that have not started are not released. Existing child Jobs are not a rollback mechanism; inspect or stop them separately as needed.

1. Open the failed DAG node and inspect its Workers, events, and container logs.
2. Check image pull access, cluster and Worker availability, selectors and resource requests, storage mounts, Domain connectivity, and the script exit code.
3. Correct the underlying configuration or workload. The current Workflow run cannot resume from the failed node; submit a new Workflow run with a unique name (or delete the old run before reusing its name).
4. Verify the replacement run with the checks above.

## API Equivalent

Create a Workflow CR with `POST /api/v1/rlinf.io/v1alpha1/workflows`, then query the Workflow and generated Jobs. See [CRD Reference](../reference/crd.md).
