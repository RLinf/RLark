import sys
import time

import ray


address = sys.argv[1]
required = int(sys.argv[2])
deadline = time.time() + 900

print("Starting Ray cluster status check...")
print(f"Waiting for cluster to have at least {required} nodes.")
print(f"Ray Address: {address}")

while time.time() < deadline:
    try:
        ray.init(
            address=address,
            ignore_reinit_error=True,
            namespace="cluster_check",
        )
        nodes = ray.nodes()
        alive = sum(1 for node in nodes if node.get("Alive"))
        print(f"Found {len(nodes)} total nodes, {alive} of which are alive.")
        if alive >= required:
            print(
                f"Cluster is ready: {alive} nodes available "
                f"(required: {required})."
            )
            sys.exit(0)
    except Exception as error:
        print(f"[Warning] Failed to connect or query Ray cluster: {error}")
    finally:
        try:
            ray.shutdown()
        except Exception:
            pass

    print(f"Time remaining: {max(0, deadline - time.time()):.0f}s")
    time.sleep(10)

print(
    "Error: Cluster did not become ready within 15 minutes "
    f"(required {required} nodes)."
)
sys.exit(1)
