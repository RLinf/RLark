# Clusters and Nodes

Use this guide to find a data-plane cluster and confirm that it has suitable Worker capacity before creating a Job.

## Task 1: Find an Available Cluster

1. Open **Clusters** in the platform console.
2. Search by cluster name or ID, or filter by the metadata shown in the list.
3. Check the online status, online rate, and Worker count.
4. Open the cluster that you plan to use.

![Cluster list](../images/ui/first-login-cluster-list.png)

## Task 2: Check Cluster Capacity

1. In cluster details, review total and offline Workers and running Jobs.
2. Compare the cloud compute, edge compute, and robot resource groups.
3. Filter or search the Worker list to find the required hardware.

![Cluster detail](../images/ui/first-login-cluster-detail.png)

## Task 3: Inspect a Worker

1. Select a Worker from the cluster details or open **Nodes** and search for it.
2. Confirm that it is online and schedulable.
3. Review its access mode, operating system, architecture, and Agent version.
4. Review CPU, memory, and GPU capacity and requested resources. The usage values are aggregated Kubernetes requests, not real-time hardware utilization.
5. Check the Jobs and Workers already placed on the node.

![Node detail](../images/ui/first-login-node-detail.png)

!!! note "Node categories"
    The platform groups Workers using RLark category labels for cloud, edge, and robot resources. Legacy category values and nodes that explicitly advertise supported resources remain visible. Kubernetes control-plane nodes are excluded from the platform Worker view.

## Result

You should now have a cluster and, when needed, a node selector that matches online, schedulable Workers with sufficient capacity. Use them in [Submit and Manage a Job](jobs.md).

## API Equivalent

Query Cluster summaries and Node CRs. See [API Reference](../api/reference.md) for fields and filters.
