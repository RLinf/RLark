# Development and Debugging

Run focused unit tests while developing, then execute repository lint and test targets before submission. Use structured component logs, Kubernetes events, and CR status transitions to trace reconciliation. Keep generated API clients in sync with CRD changes and test user-facing changes in both the API and Web UI.

## Frontend Data Modes

The Web UI uses `VITE_DATA_MODE` to select its data source. Development defaults to `mock`; integration and production use `backend`. Mock data is only for UI development and documentation screenshots and must not be treated as real resource state. Explicitly use backend mode when validating clusters, nodes, Jobs, storage, or capacity:

```bash
cd apps/rlark-ui
VITE_DATA_MODE=backend npm run dev
```

Language, theme, and sidebar state are stored in the browser. Lists and details use stable URLs for refresh, sharing, and back navigation. These are frontend implementation and debugging conventions, not platform user procedures.
