import argparse
import sys
import time
import signal
from typing import Optional

import ray


def signal_handler(signum, frame):
    print(f"Signal {signum} received, shutting down gracefully...")
    if ray.is_initialized():
        ray.shutdown()
    sys.exit(0)


def get_current_node_count(ray_address: str) -> Optional[int]:
    try:
        ray.init(address=ray_address, ignore_reinit_error=True, namespace="cluster_check")
        nodes = ray.nodes()
        alive_nodes = sum(1 for node in nodes if node.get("Alive"))
        print(f"Found {len(nodes)} total nodes, {alive_nodes} of which are alive.")
        return alive_nodes
    except Exception as e:
        print(f"[Warning] Failed to connect or query Ray cluster: {e}")
        return None
    finally:
        try:
            if ray.is_initialized():
                ray.shutdown()
        except:
            pass


def wait_for_nodes(
    ray_address: str,
    required_nodes: int,
    timeout_seconds: int = 900,
    retry_interval_seconds: int = 10,
) -> bool:
    start = time.time()
    print(f"Waiting for cluster to have at least {required_nodes} nodes.")
    print(f"Ray Address: {ray_address}")

    while time.time() - start < timeout_seconds:
        current = get_current_node_count(ray_address)
        if current is not None:
            print(f"Current node count: {current}")
            if current >= required_nodes:
                print(
                    f"Cluster is ready: {current} nodes available "
                    f"(required: {required_nodes})."
                )
                return True
        else:
            print("Could not retrieve node count, retrying...")

        remaining = timeout_seconds - (time.time() - start)
        print(f"Time remaining: {remaining:.0f}s")
        time.sleep(retry_interval_seconds)

    print(
        f"Error: Cluster did not become ready within "
        f"{timeout_seconds // 60} minutes "
        f"(required {required_nodes} nodes)."
    )
    return False


def main():
    signal.signal(signal.SIGTERM, signal_handler)
    signal.signal(signal.SIGINT, signal_handler)

    parser = argparse.ArgumentParser(
        description="Wait for a Ray cluster to reach the required number of nodes."
    )
    parser.add_argument("ray_address", type=str, help="The address of the Ray cluster.")
    parser.add_argument(
        "required_nodes", type=int, help="The required number of nodes."
    )
    args = parser.parse_args()

    print("Starting Ray cluster status check...")
    success = wait_for_nodes(
        ray_address=args.ray_address,
        required_nodes=args.required_nodes,
    )

    if success:
        print("Ray cluster check passed.")
        sys.exit(0)
    else:
        print("Ray cluster check failed.")
        sys.exit(1)


if __name__ == "__main__":
    main()
