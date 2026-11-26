# Console Access and Navigation

## Signing In

RLark has two login entry points:

| Entry            | URL                        | Purpose                                                       |
| ---------------- | -------------------------- | ------------------------------------------------------------- |
| Platform Console | `http://<host>:5173`       | Job management, Workers, storage, SSH keys                    |
| Admin Console    | `http://<host>:5173/admin` | Cluster onboarding, nodes, certificates, system configuration |

### Login Steps

1. Open the console URL in your browser
2. Enter your username and password
3. Click Login
4. The Gateway issues a JWT access token (8 hours by default), and the browser sends it as a Bearer token on subsequent API requests

!!! warning "Role authorization is coarse-grained"
    Gateway enforces `admin` access for control-plane configuration and credential management. The `user` role can manage platform workloads, but per-user resource ownership is not yet implemented, so authenticated users are not isolated from each other's Jobs or storage objects. Continue to use TLS and network controls.

## Console Navigation

In the Astribot edition, the sidebar shows the Astribot logo, and the RLark logo appears by **RLark Open Source** in the footer.

### Platform Console Pages

| Page     | Purpose                                                        |
| -------- | -------------------------------------------------------------- |
| Overview | Dashboard summary of clusters, schedulable workload nodes, robots, and running jobs |
| Clusters | Browse and inspect onboarded data-plane clusters               |
| Nodes    | Filter and inspect node resources, scheduling, and health      |
| Jobs     | Create, monitor, and manage training jobs                      |
| Storage  | Browse storage classes and object storage                      |
| SSH Keys | Manage public SSH keys for Worker access                       |

### Admin Console Pages

The **Image Registries** page stores private-registry credentials and selects their distribution scope: store only, selected clusters, or all current and future clusters. Display names may be duplicated and edited; RLark uses an internal ID to identify each credential. Distribution and deletion are asynchronous. Delivered credentials are placed in `rlark-system` on each selected Kubernetes data plane.

> **Screenshot note:** Screenshots are from an example environment. Resource names and data are illustrative; your environment will differ.

![Console overview](../images/ui/console-overview.png)

### Node Removal Progress

On **Nodes**, an administrator starts removal with the row's **Remove** action. A centered confirmation dialog summarizes the node, cluster, IP address, status, and category. It lists the removal steps and up to six affected workloads, and calls out an unreachable site machine or an unverified node identity when applicable. Confirming creates a node operation on the control plane. Closing the dialog or leaving the page does not cancel it: the row shows **Removing** with the current stage, and a task card in the lower-right corner follows the operation. The card collapses and opens a detail drawer with every stage and its condition. Reloading the page restores progress from the operation list. If a stage fails, or the member cluster API cannot be reached, the detail drawer shows the reason and offers **Retry**. While a removal is pending or running, the console does not submit a second one for the same node; the Gateway also rejects a duplicate with `409 NODE_OPERATION_IN_PROGRESS`. A control-plane node cannot be removed.

### Site Node Details

The **Site Nodes** page is a bounded content column (1480px) centered on wide screens. **Add node** is in the toolbar, and the status tabs are quiet underline tabs showing the All, Bound, Offline, Conflict, and Retired counts. The toolbar searches node names and UIDs, filters by member cluster, refreshes the list, and opens recent node operations for the selected cluster. Each cluster with robots has its own section headed by the cluster name and a mono stat line (drivers ready, camera issues, online): its control-plane node is a compact row and its robots are camera-wall tiles in the same section. A robot tile puts the picture first: the identity, status, verification tags, desired and current power, and the controls that fit the current state (start or stop the driver, power on or off, hold-to-move home pose) sit directly on the large camera view, the smaller views sit in a strip under it, and a slow preview refreshes every five seconds. A tile whose power faults, e-stops, fails its self-check, or is under maintenance gets an amber ring so trouble reads from across the room. The full detail has all four power and driver actions. A power or driver command shows that it is being sent and then whether the Gateway accepted it; the desired state changes at once and the robot catches up on its next report, with a wait explained in its Node's `rlark.io/power` annotation. The tile's current power also shows a hardware fault or an unreadable vendor monitor instead of leaving the value blank, and the open detail shows the Node's `rlark.io/power` annotation when the power service reports one, so a blocked pose can be traced. The home-pose control is hold-to-move: its label says so, a short click explains that it must be held, an unavailable pose says whether the robot needs power, the driver, an emergency-stop reset, or reports a hardware fault, and releasing it reports the stop or the refusal. Selecting robots reveals the existing bulk power and pose controls. Other clusters are quiet row lists below — icon, name, admission state, a dot status word, and mono facts per row, separated by hairlines instead of card boxes. Every detail starts collapsed, and only one detail is open at a time; the detail opens as a dialog over the page, so it never stretches or moves the tiles and rows. The tile grid drops to one column on phones.

**Open details** opens the station in a dialog over the page instead of a drawer or an in-card tab; the tiles and rows stay where they are. It places identity, network and admission, and conditions side by side; a robot's dialog adds its power and driver actions and a live four-camera strip. Escape or the close button closes it, and clicking the dimmed area outside does not. A conflict record shows both the UID recorded for it and the UID currently reported under the name. Values such as node UID, WireGuard key, tunnel address, and SSH host fingerprint can be copied. Reissue, revoke, and **Delete record** are in the panel header, with the reason shown when an action is unavailable; **Delete record** is enabled only when the record is eligible. Clicking a camera opens an enlarged preview and returns focus to that camera button when it closes. Escape closes the details and returns focus to the tile's expand button. Deleting a site-node record does not remove a node that still reports under the name; it registers again with its next report.

On wide dialogs, **Identity**, **Network and admission**, and **Conditions** use equal-width columns. Narrow dialogs show fewer columns so long values stay readable.

**Site network links** summarizes connectivity below the three sections. Expand it to see the state and timestamps reported by each Hub.

Expand **Running container images** in a node's details to see the namespace, Pod, container, reported image, explicit tag when present, and runtime image ID when reported. This is an administrator-only snapshot of containers Kubernetes reports as running on that node. It excludes images that are only cached on disk and containers started outside Kubernetes, such as standalone Docker containers. Use **Refresh** for a new snapshot. If the member cluster cannot be reached or the node identity no longer matches the site record, the console shows that images are unavailable.

The **Admission records** section lists code and SSH admissions with their cluster, state, issuer, issue and expiry times, WireGuard key, and tunnel address. A pending code can be revoked, an admission can be reissued, a redeemed but unfinished join can be cleared to start over, and an SSH join can be opened from its record. The code itself is not stored or shown again after issuance.

### Joining Nodes

Open **Site Nodes → Add node**. The page has a compact header, a target-cluster selector, and a three-way method switch: discovered machines, SSH, or a one-time join code. The machine workspace has **All / Discovered / Manual** source filters.

- **Discovered** lists the machines the site's hubs found. They can be searched and selected in a batch; **Join selected machines** opens a password dialog instead of submitting at once. The dialog groups the selection by NodeClass, shows whether the Orin entry-device password is configured, and accepts an x86 login password plus an optional sudo password. Leaving the x86 field empty keeps the stored class credential; typing a value replaces and saves it. The dialog writes the class credentials before it admits the machines. These passwords are class credentials, not one-time secrets; RLark shows only whether one is stored, never its value.
- **Manual** lists SSH joins that are still running or stopped; **Open SSH record** switches to the SSH method with that record selected. **Add computer/GPU** opens the SSH form directly; after submission the machine appears under Manual. Ordinary x86/GPU servers are never produced by site discovery; use SSH or a join code for them.
- The operations rail follows batch, SSH, and code joins and continues on the server when the page is left. On a wide screen it is the right-hand column; below that width it follows the workspace. The SSH and join-code methods keep their own forms, a short flow and notes column, and their recent records.

### Discovery Settings

Open **Administrator Console → Site Nodes → Discovery Settings** (`/admin/stations/network`), or select **Discovery Settings** from **Add node**. The page is administrator-only: the Gateway rejects reading and saving with `403`, and the console shows a read-only notice. A `404` means the site network has yet to be initialized.

The header carries the network health strip (Ready, FullyConnected, DiscoveryStale, observed/current generation) and **Back to joining** / **Refresh**. The workbench is one surface:

- The left rail selects a site. The editor shows the selected site's status, clusters, and scanning Hubs; **Cancel** and **Save settings** sit in the editor header, and the buttons stay disabled until the draft changes.
- Scanning can be enabled or disabled, the interval set to 10–3600 seconds, and 1–8 unique extra IPv4 prefixes added with lengths from `/22` to `/32`. A management-plane dial-out Hub (`from`) is chosen separately from the scanning Hubs. Only the selected site's enrolled Hubs scan.
- The right-hand details expose the site's clusters and Hubs, the discoverable NodeClasses with their entry port and access/discovery flags, and the factory host-key fingerprints. On narrower screens these details move below the editor.
- The editor header marks unsaved changes; on a `409` conflict the page reloads the latest configuration and asks you to review it before saving again. Unsaved edits are kept per site when switching the site rail.
- **Scanning scope and limits** is a collapsed shared panel. The five rules are unchanged: only entry devices matching `NodeClass.spec.discovery.hostKeyFingerprints` are discovered; hubs also scan their connected private networks; overlapping ranges may duplicate Machines; changes take about 30 seconds; a site without an enrolled Hub does not scan.

Below the workbench, three status cards summarize the current site network without replacing the management dashboard:

- **Recent scan activity** lists recently discovered machines and settings saves.
- **Site discovery load** counts the machines each site has found; selecting a site switches the editor to it.
- **Astribot robots** reads site-node data for online robots, power states, camera health, machines being joined, and per-site online counts; **View site nodes** opens the Site Nodes page.

Ordinary x86/GPU servers do not have a discovery declaration and do not appear in the scan list. They join over SSH or with a join code; scanning them directly would require a separate direct/entry-only admission mode.

### Creating a Cluster

The **New Cluster** page collects the site and its first machine in two sections and keeps a **What will be created** summary beside the form. Required fields are marked, and invalid input is explained under the field instead of only disabling the button. Enter submits the form. After submission, the progress card shows the provisioning stage and earlier attempts; leaving the page does not cancel provisioning.

### Finding the Right Page

| Question                                       | Go to                              |
| ---------------------------------------------- | ---------------------------------- |
| Is a cluster or node available for scheduling? | Clusters or Nodes page             |
| How to create a training job?                  | Jobs → Create Job                  |
| Which stage is a job stuck at?                 | Job Details → Workers tab          |
| How to find application errors?                | Job Details → Logs tab             |
| How to check resource usage?                   | Nodes page → Node detail           |
| How to open a terminal in a container?         | Job Details → Worker → WebTerminal |
| How to configure private image credentials?    | Admin Console → Image Registries   |

### Refreshing Console Data

Use **Refresh** to request the latest data without leaving the current page. While the request is in progress, the affected table or dashboard is dimmed, its controls are temporarily disabled, and a centered progress indicator is shown. Existing rows remain visible until the refreshed response arrives. This behavior is used consistently across overview dashboards, clusters and nodes, domains, jobs and Workers, storage and files, SSH keys, image registries, and system configuration.

### Interface Language

The language switch in the header applies to both Chinese and English. Besides static labels, it also localizes values that the console derives from control-plane state: node status reasons, Addon phases, the CPU core unit, derived node kinds in Worker filters, and list separators. The login page and the WebTerminal page follow the same switch. Values passed through verbatim from the data plane — node labels, operating system and architecture identifiers, and other server-provided strings — are shown as reported and are not translated.

### Long Values in Node Tables

In the node detail Worker table, long instance names are truncated with an ellipsis, and in the expanded Pod sub-table the network domain is truncated the same way. Hover over a truncated value to see the full text in a tooltip. This keeps the status column visible without horizontal scrolling.

### Filtering Jobs by Tag

The tag filter in the job list combines the selection the same way the Gateway does: different tag keys are ANDed, and several values of the same key are ORed. A job is listed only when it carries every selected key, and for each of those keys at least one of the selected values. Selecting keys that no single job carries together therefore returns an empty list; this is the filter semantics, not a failed request. The popover states the rule above the columns, and the **All / None** shortcut applies to the values of the key currently in focus, never to the key list.

A key comes into focus when you click its row, and the value column keeps showing that key until you click another one. Hovering does not change the focus, so moving the pointer towards the values never replaces the list. The checkbox in front of a key is a separate control: it selects or clears all values of that key at once.

### Role Count in the Job List

The **Roles** column counts the distinct role names of the Job, the same names the job detail page and the job editor show. It is not derived from `Task.spec.role`, which is only a coarse grouping hint (`Actor`, `Rollout`, `Env`) and collapses several business roles into one value.

### Copy Buttons

Copy buttons report success inline instead of silently succeeding: the icon switches to a check mark for about 1.6 seconds and then returns to the copy icon. The image copy button in the job detail role runtime panel also turns green with a short pulse and changes its tooltip to **Copied**. A button that stays unchanged means the copy did not happen, usually because the browser blocked clipboard access.

Every copy button in the console shares one clipboard path. Browsers expose the async clipboard API only in a secure context (HTTPS or `localhost`), so when the console is served over plain HTTP the copy automatically falls back to a compatible path instead of failing. This covers the SSH key, SSH connect command, Worker SSH command, API example, deployment YAML, workflow YAML, and job log copy buttons. Success feedback appears only after the copy actually succeeds.

### Closing Form Dialogs

Dialogs that collect input — creating, cloning, editing, or restart-editing a job, creating or editing a storage class, adding or editing image registry credentials, adding an SSH key, creating a domain, and editing job tags — do not close when you click the dimmed area outside the dialog. Because the console cannot restore discarded input, these dialogs close only through their close button in the upper-right corner or their **Cancel** button; the job, storage class, and image registry dialogs also close with `Esc`.

Confirmation dialogs that collect no input, such as job lifecycle actions, job deletion, the restart mode choice, and cluster lifecycle actions, still close when you click outside them.

## API Equivalent

Supported resource operations are available through the [Gateway API](../api/reference.md). The standalone Gateway defaults to `http://<host>:8080`; an `rlarkadm` deployment uses its configured Service and UI proxy. The API reference is authoritative because not every UI interaction has a one-to-one public endpoint.
