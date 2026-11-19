set -eu
unshare -n sleep 90 & client=$!
unshare -n sleep 90 & server=$!
trap 'kill "${other:-}" "${responder:-}" "$client" "$server" 2>/dev/null || true' EXIT
ip link add wg-test type veth peer name client-veth
ip link add eth-test type veth peer name server-veth
ip link set client-veth netns "$client"
ip link set server-veth netns "$server"
ip addr add 10.254.240.1/24 dev wg-test
ip addr add 10.11.14.128/16 dev eth-test
ip link set wg-test up
ip link set eth-test up
nsenter -t "$client" -n ip link set lo up
nsenter -t "$client" -n ip addr add 10.254.240.4/24 dev client-veth
nsenter -t "$client" -n ip addr add 10.254.240.5/24 dev client-veth
nsenter -t "$client" -n ip link set client-veth up
nsenter -t "$client" -n ip route add default via 10.254.240.1
nsenter -t "$server" -n ip link set lo up
nsenter -t "$server" -n ip addr add 10.11.5.12/16 dev server-veth
nsenter -t "$server" -n ip link set server-veth up
sysctl -w net.ipv4.ip_forward=1 >/dev/null
STATION_NETNS_TEST=1 GOWORK=off go test -count=1 -run TestFirewallIdempotence ./...
nsenter -t "$server" -n python3 -u -c 'import socket;s=socket.socket();s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(("10.11.5.12",445));s.listen();
while True:
 c,a=s.accept();c.sendall(a[0].encode());c.close()' & responder=$!
sleep 1
nsenter -t "$client" -n python3 -c 'import socket;s=socket.socket();s.settimeout(3);s.bind(("10.254.240.4",0));s.connect(("10.11.5.12",445));assert s.recv(100)==b"10.11.14.128";print("Allowed SMB TCP forwarded with expected SNAT")'
nsenter -t "$client" -n python3 -c 'import socket;s=socket.socket();s.settimeout(2);s.bind(("10.254.240.5",0));
try: s.connect(("10.11.5.12",445));raise SystemExit("unexpectedly allowed")
except TimeoutError: print("Unlisted source denied")'

nsenter -t "$server" -n python3 -u -c 'import socket;s=socket.socket();s.bind(("10.11.5.12",446));s.listen();s.accept()' & other=$!
sleep 1
nsenter -t "$client" -n python3 -c 'import socket;s=socket.socket();s.settimeout(2);s.bind(("10.254.240.4",0));
try: s.connect(("10.11.5.12",446));raise SystemExit("unexpected port allowed")
except TimeoutError: print("Unlisted destination port denied")'
