package release

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Result is one check's verdict and what it read, in one line: the objects
// at fault when it failed, a count when it did not.
type Result struct {
	OK      bool
	Reading string
}

// Check compares the plane with the baseline taken before the batch. It fails for what got worse, never for what was
// already bad: the site has an office node that is off and Addons that sit in Installing on a good day. written are
// the Addons a run has written, each to the version it wrote ("<clusterID>/<addon>" -> version): an Addon at that
// version is as the run made it, at any other version it changed beside the run.
type Check func(baseline, current Snapshot, written map[string]string) Result

// Checks is every check a stage can name by itself.
var Checks = map[string]Check{
	"enrollments-enrolled": enrollmentsEnrolled,
	"documents-unchanged":  documentsUnchanged,
	"links-connected":      linksConnected,
	"network-connected":    networkConnected,
	"clusters-online":      clustersOnline,
	"nodes-ready":          nodesReady,
	"addons-ready":         addonsReady,
	"observations-fresh":   observationsFresh,
	"nodes-unpressured":    nodesUnpressured,
}

// LevelChecks are the checks a stage names with a level, "<name>: <level>".
// They read the plane as it is: what a batch needs of a node does not depend
// on what the node was before it.
var LevelChecks = map[string]func(current Snapshot, level int32) Result{
	"gateways-understand": gatewaysUnderstand,
}

// Verify runs the check a condition names.
func (c Condition) Verify(baseline, current Snapshot, written map[string]string) Result {
	if c.Level > 0 {
		return LevelChecks[c.Check](current, c.Level)
	}
	return Checks[c.Check](baseline, current, written)
}

func result(faults []string, healthy string) Result {
	if len(faults) == 0 {
		return Result{OK: true, Reading: healthy}
	}
	slices.Sort(faults)
	return Result{Reading: strings.Join(faults, "; ")}
}

func enrollmentsEnrolled(baseline, current Snapshot, _ map[string]string) Result {
	var faults []string
	enrolled := 0
	for id, was := range baseline.Enrollments {
		if was.Phase != "Enrolled" {
			continue
		}
		enrolled++
		if phase := current.Enrollments[id].Phase; phase != "Enrolled" {
			faults = append(faults, fmt.Sprintf("%s was Enrolled, is %s", id, cmp.Or(phase, "absent")))
		}
	}
	for id, now := range current.Enrollments {
		for _, problem := range now.Problems {
			if !slices.Contains(baseline.Enrollments[id].Problems, problem) {
				faults = append(faults, fmt.Sprintf("%s has a new problem: %s", id, problem))
			}
		}
	}
	return result(faults, fmt.Sprintf("%d of %d enrollments Enrolled as before, no new problems", enrolled, len(baseline.Enrollments)))
}

func documentsUnchanged(baseline, current Snapshot, _ map[string]string) Result {
	short := func(hash string) string { return cmp.Or(hash[:min(12, len(hash))], "none") }
	var faults []string
	appeared := 0
	for id, was := range baseline.Enrollments {
		now := current.Enrollments[id].ConfigHash
		switch {
		case now == was.ConfigHash:
		case was.ConfigHash == "":
			// A node that had no document when the run began has one now: it came back, nothing it ran was changed.
			appeared++
		default:
			faults = append(faults, fmt.Sprintf("%s document %s -> %s", id, short(was.ConfigHash), short(now)))
		}
	}
	return result(faults, fmt.Sprintf("%d documents as before, %d that had none have one", len(baseline.Enrollments)-appeared, appeared))
}

func linksConnected(baseline, current Snapshot, _ map[string]string) Result {
	// Unknown claims nothing: the Hub has no current report of the Leaf. It
	// fails a link only when the link was Connected.
	down := func(state string) bool { return state == "Unanswered" || state == "Silent" }
	var faults []string
	connected := 0
	for id, was := range baseline.Enrollments {
		for hub, state := range was.Links {
			if state != "Connected" {
				continue
			}
			connected++
			if now := current.Enrollments[id].Links[hub]; now != "Connected" {
				faults = append(faults, fmt.Sprintf("%s -> %s was Connected, is %s", id, hub, cmp.Or(now, "absent")))
			}
		}
	}
	for id, now := range current.Enrollments {
		for hub, state := range now.Links {
			if was := baseline.Enrollments[id].Links[hub]; down(state) && was != "Connected" && !down(was) {
				faults = append(faults, fmt.Sprintf("%s -> %s was %s, is %s", id, hub, cmp.Or(was, "absent"), state))
			}
		}
	}
	return result(faults, fmt.Sprintf("%d links Connected as before, none newly Unanswered or Silent", connected))
}

func networkConnected(baseline, current Snapshot, _ map[string]string) Result {
	was, now := baseline.SiteNetwork, current.SiteNetwork
	var faults []string
	if was.Ready && !now.Ready {
		faults = append(faults, "SiteNetwork is no longer Ready")
	}
	if was.FullyConnected && !now.FullyConnected {
		faults = append(faults, "SiteNetwork is no longer FullyConnected")
	}
	for _, pair := range now.Unreachable {
		if !slices.Contains(was.Unreachable, pair) {
			faults = append(faults, "no path "+pair)
		}
	}
	return result(faults, fmt.Sprintf("Ready=%t FullyConnected=%t, %d members, %d pairs without a path, none new", now.Ready, now.FullyConnected, now.Members, len(now.Unreachable)))
}

func clustersOnline(baseline, current Snapshot, _ map[string]string) Result {
	// Cordoned, Draining and Delisted are what an administrator declared;
	// they rank with Offline, so a cluster that was in one of them fails
	// nothing and a cluster that was serving and no longer is does.
	rank := map[string]int{"Online": 2, "Downgraded": 1}
	var faults []string
	for id, was := range baseline.Clusters {
		if now := current.Clusters[id]; rank[now] < rank[was] {
			faults = append(faults, fmt.Sprintf("cluster %s was %s, is %s", id, was, cmp.Or(now, "absent")))
		}
	}
	phases := make([]string, 0, len(current.Clusters))
	for id, phase := range current.Clusters {
		phases = append(phases, id+" "+phase)
	}
	slices.Sort(phases)
	return result(faults, "none worse than before: "+strings.Join(phases, ", "))
}

func nodesReady(baseline, current Snapshot, _ map[string]string) Result {
	var faults []string
	online := 0
	for id, was := range baseline.Nodes {
		if was != "Online" {
			continue
		}
		online++
		if now := current.Nodes[id]; now != "Online" {
			faults = append(faults, fmt.Sprintf("node %s was Online, is %s", id, cmp.Or(now, "absent")))
		}
	}
	return result(faults, fmt.Sprintf("%d of %d nodes Online as before", online, len(baseline.Nodes)))
}

func addonsReady(baseline, current Snapshot, written map[string]string) Result {
	var faults []string
	ready := 0
	for id, was := range baseline.Addons {
		now, found := current.Addons[id]
		if was.Phase == "Ready" {
			ready++
			if now.Phase != "Ready" {
				faults = append(faults, fmt.Sprintf("addon %s was Ready, is %s", id, cmp.Or(now.Phase, "absent")))
			}
		}
		if found && now.Version != was.Version && (written[id] == "" || written[id] != now.Version) {
			faults = append(faults, fmt.Sprintf("addon %s version %s -> %s", id, cmp.Or(was.Version, "none"), cmp.Or(now.Version, "none")))
		}
	}
	return result(faults, fmt.Sprintf("%d of %d addons Ready as before, versions as before or as allowed", ready, len(baseline.Addons)))
}

func observationsFresh(baseline, current Snapshot, _ map[string]string) Result {
	var faults []string
	fresh := 0
	for id, was := range baseline.Observations {
		if !was.Fresh {
			continue
		}
		fresh++
		now, found := current.Observations[id]
		switch {
		case !found:
			faults = append(faults, id+" has no observation")
		case !now.Fresh:
			faults = append(faults, fmt.Sprintf("%s last reported %s ago", id, current.TakenAt.Sub(now.ReportedAt).Round(time.Second)))
		}
	}
	return result(faults, fmt.Sprintf("%d of %d nodes reported in the last %s as before", fresh, len(baseline.Observations), ObservationFresh))
}

func nodesUnpressured(baseline, current Snapshot, _ map[string]string) Result {
	var faults []string
	reporting := 0
	for id, now := range current.Observations {
		// A report that is not fresh says what the Node was under then.
		if !now.Fresh || now.Pressures == nil {
			continue
		}
		reporting++
		was := baseline.Observations[id].Pressures
		for _, pressure := range now.Pressures {
			switch {
			case slices.Contains(was, pressure):
			case was == nil:
				// Not known to be new, and not known to be old.
				faults = append(faults, fmt.Sprintf("%s is under %s; the baseline has no reading of it", id, pressure))
			default:
				faults = append(faults, fmt.Sprintf("%s is under %s, was not", id, pressure))
			}
		}
	}
	return result(faults, fmt.Sprintf("%d nodes under no pressure they were not under before; %d not read: their last fresh report has none", reporting, len(current.Observations)-reporting))
}

// gatewaysUnderstand holds when every member that is given a document runs a
// gateway that states a level of at least the one asked for. A gateway that
// states none is not reconciling, and is not known to understand anything.
// The last report counts however old it is, because a node that is off comes
// back with the gateway it had.
func gatewaysUnderstand(current Snapshot, level int32) Result {
	var faults []string
	members := 0
	for id, enrollment := range current.Enrollments {
		if enrollment.ConfigHash == "" {
			continue
		}
		members++
		switch has := current.Observations[id].GatewayLevel; {
		case has == 0:
			faults = append(faults, id+" gateway states no level")
		case has < level:
			faults = append(faults, fmt.Sprintf("%s gateway understands level %d", id, has))
		}
	}
	return result(faults, fmt.Sprintf("the gateways of all %d members with a document understand level %d", members, level))
}
