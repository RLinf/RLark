# Cluster Management Changes

## Scope

This document summarizes the cluster-management development changes. It describes development work, not a published RLark release.

## Summary

The branch introduces a cluster-scoped `Cluster` custom resource as the authoritative representation of a registered data-plane cluster. Cluster status is now reconciled from Agent and Node heartbeats, and Gateway cluster APIs and the web UI consume the resource rather than inferring a cluster solely from grouped Node records.

## Changes

### Cluster API and generated clients

- Adds the `clusters.rlinf.io` cluster-scoped CRD, including typed clients, apply configurations, informers, listers, and deep-copy support.
- Defines durable Cluster intent through `spec.agentNamespace`, `spec.cordoned`, and `spec.draining`.
- Adds Cluster lifecycle phases: `Online`, `Offline`, `Downgraded`, `Cordoned`, and `Draining`.
- Reports the latest Agent heartbeat and aggregate Worker counts in Cluster status.

### Reconciliation and Agent registration

- Adds a Cluster controller that reconciles status from the cluster Agent heartbeat Lease and Nodes in `spec.agentNamespace`.
- Uses periodic requeues to repair missed watch events and reports offline or downgraded states when heartbeats or individual Workers are unavailable.
- Creates a Cluster during Agent registration and updates the legacy cluster ID label on Nodes in the Agent management namespace only as a compatibility lookup value.
- Distinguishes cluster Agent heartbeats from node Agent heartbeats. Node heartbeats use `node-heartbeat-<node-name>` Leases.
- Adds node sweeping so management Nodes no longer present or enabled in the local cluster are marked offline.

### Gateway and UI

- Changes cluster list and detail endpoints to list and retrieve Cluster custom resources, so empty registered clusters remain visible.
- Returns the controller-managed Cluster phase instead of deriving a phase inside the Gateway.
- Updates cluster-management, cluster-list, node-browser, and job-creation UI flows to use the revised Cluster and Node status information.

### Documentation and tests

- Documents Cluster and Worker heartbeat behavior, lifecycle phase meanings, and management expectations in the user and administrator guides.
- Regenerates the CRD schema reference for the new Cluster resource.
- Adds or updates controller, Agent, Gateway, component, and UI-related tests for cluster status, heartbeat, migration, and offline detection behavior.

## Operational Notes

- This branch is development work and does not establish a stable release or upgrade guarantee.
- A cluster is considered offline when its `heartbeat` Lease is missing or expired. Agent heartbeat Leases renew every 15 seconds and expire after 45 seconds by default; a registered Worker is considered offline when its corresponding node heartbeat Lease is missing or expired.
- `cordoned` and `draining` are durable desired-state fields. They take precedence over heartbeat-derived cluster phases.

## Validation Basis

The summary was derived from the updated API types, user documentation, generated CRD reference, and focused test changes.
