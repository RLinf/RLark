# Web console behavior

The RLark console separates live backend data from demonstration content:

- Resource totals, health, notifications, cluster lists, node lists, jobs, and storage views use backend responses when the gateway is available.
- When the gateway is unavailable, the console labels the session as **Mock** and uses the same fallback node dataset across overview and node pages.
- The China resource map is explicitly marked as **Demo data**. Its locations and links are illustrative until node city metadata is available from the backend.

Navigation uses stable list and detail URLs. Returning from a detail view replaces that detail history entry, while selecting another resource creates a normal browser-history entry. Map city and resource-type controls open the node list with the corresponding filter.

Language, theme, and sidebar preferences are stored locally in the browser. Dialogs can be dismissed with Escape when no submission is in progress, and future workflow steps remain locked until the preceding step has been reached.

