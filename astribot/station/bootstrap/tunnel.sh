#!/bin/sh
# The site tunnel of a machine that reaches Station API only through it: a
# node the SiteNetwork declares with its key (a new site's first machines).
# station-bootstrap runs it as a host process:
#
#   station-bootstrap tunnel
#
# The machine's key is the one "station-gateway --public-key" made, which the
# declaration holds; its document is the one the SiteNetwork's status
# publishes for the node (status.declared), put by the administrator at
# /etc/rlark/station/bootstrap.json. Once a peer answers, the operator's join command
# for the node redeems its code through the tunnel, or "control-plane" makes
# the machine its cluster's control plane. Run again, it changes nothing.
set -eu
. /var/lib/rlark/bootstrap/lib.sh
: "${TUNNEL_INTERFACE:=st-wg0}"
D=/etc/rlark/station/bootstrap.json
[ -s $D ] || fail "no document at $D: the SiteNetwork's status.declared holds this node's, for its key ($B/bin/station-gateway --public-key)"
install -d /opt/rlark/bin
install -m 755 "$B"/bin/* /opt/rlark/bin/
tunnel_service
# Whether its peers have taken its key and answer is for the caller to wait for
# (the Gateway reads their reports, for an SSH join and a new cluster; station-gateway --await-peers).
stamp "tunnel service started on $TUNNEL_INTERFACE: $(ip -4 -o addr show dev "$TUNNEL_INTERFACE" | awk '{print $4}')"
