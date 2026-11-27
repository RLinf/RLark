"""The S1U robot node's specification (user 11-13: "任意一台s1u机器人新进来，和之前的都是一样的……百台机器人要完全
一致"): every item a robot x86 must agree on with every other, who makes it so, and whether this node conforms. The
node tuner (node_tuner.py) evaluates it every minute and publishes on its Node

    label       rlark.io/node-conformance             ok | not-ok
    annotation  rlark.io/node-conformance-report      each item that does not hold, with who converges it
    annotation  rlark.io/node-fingerprint-model       the same on every S1U of every site
    annotation  rlark.io/node-fingerprint-site        the same on every robot of a site
    annotation  rlark.io/node-fingerprint-body        the same on every robot of a body type
    annotation  rlark.io/node-facts                   the values the fingerprints are of
    annotation  rlark.io/node-spec                    the digest of this file and the node profile

A fingerprint is of configuration that should be equal, never of what is one node's (its name, addresses, serial
numbers, host keys) or changes by itself (a clock's offset, whether the Orin answers): the latter decide conformance
only. An item belongs to exactly one mechanism, which alone holds its expected value:

    node-profile    the node tuner makes the host follow node_profile.env (node_profile.sh status)
    orin, clock     the tuner Pod's other containers (their state labels)
    vendor          delivered with the robot; nobody converges it, EXPECTED states the one version in use
    admission       the labels RLark puts on the Node from the robot's admission record
    installer, station, registry-trust   RLark's; each says on the Node what it wants and whether it holds
                    (REFERENCES), and this file repeats neither its values nor its checks

Standard library only."""
import hashlib

CONFORMANCE, REPORT = "rlark.io/node-conformance", "rlark.io/node-conformance-report"
FINGERPRINT, FACTS, SPEC = "rlark.io/node-fingerprint-", "rlark.io/node-facts", "rlark.io/node-spec"
ORIN, CLOCK = "s1u.rlark.io/orin-state", "rlark.io/clock-state"  # the orin and clock containers' state labels
# What the vendor delivers and what must not be there, by the name node_profile.sh facts gives it: (value, owner). The
# container runtime is the installer's (REFERENCES), its version among the model's facts the fingerprint is of.
EXPECTED = {
    "os": ("ubuntu 20.04", "vendor"),
    "kernel": ("5.15.0-139-generic", "vendor"),
    # Images are pulled with the credentials RLark attaches to a Pod; a robot keeps none of its own.
    "registry credentials on the node": ("none", "registry-trust"),
}
# What another mechanism converges: (item, mechanism, the Node annotation of what it wants, of whether it holds), the
# names as each mechanism's owner set them (11-13). It holds when the second reads "ok" or "applied <the first's
# value>". A mechanism that has not written its pair yet counts as not conforming ("not declared").
REFERENCES = [
    # The runtime's layout, kubelet's version and arguments, the name servers kubelet gives its Pods: one pass of
    # RLark's installer, "<its package>.<digest of the site's settings>".
    ("container runtime, kubelet and name servers", "installer", "rlark.io/node-install-spec", "rlark.io/node-install-state"),
    ("site tunnel", "station", "station.rlark.io/network-spec", "station.rlark.io/network-state"),
    ("labels of the admission record", "admission", "rlark.io/node-labels-spec", "rlark.io/node-labels-state"),
    ("registry trust", "registry-trust", "rlark.io/registry-trust.spec", "rlark.io/registry-trust.state"),
]
LABELS = ["rlark.io/robot-model", "rlark.io/robot-id", "rlark.io/node-category", "rlark.io/robot-body"]


def parse(text):
    """node_profile.sh facts' lines "<scope> <name>=<value>" as {scope: {name: value}}."""
    facts = {}
    for line in text.splitlines():
        scope, _, rest = line.partition(" ")
        name, _, value = rest.partition("=")
        facts.setdefault(scope, {})[name] = value
    return facts


def conformance(node, facts, labels, annotations, profile_state, time_source):
    """(what does not hold, as "item (mechanism): detail"; the annotations of the three fingerprints) of ``node``:
    ``facts`` as parse gives them, the Node's ``labels`` and ``annotations``, the node profile's state and the site's
    time source ("" where the site has none)."""
    failing = []
    if profile_state != "ok":
        failing.append("node profile (node-profile): %s" % profile_state)
    values = {name: value for scope in facts.values() for name, value in scope.items()}
    for name, (expected, owner) in EXPECTED.items():
        if values.get(name) != expected:
            failing.append("%s (%s): %s, specified %s" % (name, owner, values.get(name), expected))
    for name in LABELS:
        if name not in labels:
            failing.append("label %s (admission): missing" % name)
    for item, owner, wanted, state in REFERENCES:
        said = annotations.get(state)
        if said is None:
            failing.append("%s (%s): not declared on the Node" % (item, owner))
        elif said != "ok" and not said.startswith("applied %s" % annotations.get(wanted)):
            failing.append("%s (%s): %s" % (item, owner, said))
    if labels.get(ORIN) != "ok":
        failing.append("Orin (orin): %s" % labels.get(ORIN))
    clock = labels.get(CLOCK)
    if clock != ("locked" if time_source else "no_source"):
        failing.append("clock (clock): %s" % clock)

    def fingerprint(scope, **more):
        items = dict(facts.get(scope, {}), **more)
        return hashlib.sha256("\n".join("%s=%s" % item for item in sorted(items.items())).encode()).hexdigest()[:16]

    wanted = {item: annotations.get(name) for item, _, name, _ in REFERENCES}
    return failing, {
        FINGERPRINT + "model": fingerprint("model", **{"wanted " + item: str(value) for item, value in wanted.items()}),
        FINGERPRINT + "site": fingerprint("site"),
        FINGERPRINT + "body": fingerprint("body", type=str(labels.get("rlark.io/robot-body"))),
    }
