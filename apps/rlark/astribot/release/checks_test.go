package release

import (
	"strings"
	"testing"
	"time"
)

var taken = time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC)

// site is a site on an ordinary day: the cloud cluster is fine, the office
// has a node that is off, an Addon that never finished installing, a link
// that was already Silent and a robot whose disk was already full.
func site() Snapshot {
	return Snapshot{
		TakenAt:     taken,
		SiteNetwork: SiteNetwork{Ready: true, Members: 3, Unreachable: []string{"cloud/gpu-1 -> office/desk-2"}},
		Clusters:    map[string]string{"cloud": "Online", "office": "Downgraded"},
		Nodes:       map[string]string{"cloud/gpu-1": "Online", "office/s1u-01": "Online", "office/desk-2": "Offline"},
		Enrollments: map[string]Enrollment{
			"cloud/gpu-1":   {Phase: "Enrolled", ConfigHash: "aaaaaaaaaaaaaaaa"},
			"office/s1u-01": {Phase: "Enrolled", ConfigHash: "bbbbbbbbbbbbbbbb", Links: map[string]string{"cloud/gpu-1": "Connected"}},
			"office/desk-2": {Phase: "Pending", ConfigHash: "cccccccccccccccc", Problems: []string{"no hub to dial"}, Links: map[string]string{"cloud/gpu-1": "Silent"}},
		},
		Addons: map[string]Addon{
			"cloud/station":  {Phase: "Ready", Version: "0.4.0"},
			"office/station": {Phase: "Installing", Version: "0.4.0"},
		},
		Observations: map[string]Observation{
			"cloud/gpu-1":   {ReportedAt: taken.Add(-20 * time.Second), Fresh: true, GatewayLevel: 2, Pressures: []string{}},
			"office/s1u-01": {ReportedAt: taken.Add(-40 * time.Second), Fresh: true, GatewayLevel: 2, Pressures: []string{"DiskPressure"}},
			"office/desk-2": {ReportedAt: taken.Add(-9 * time.Hour), GatewayLevel: 2, Pressures: []string{}},
		},
	}
}

func TestChecks(t *testing.T) {
	enrollment := func(s *Snapshot, id string, change func(*Enrollment)) {
		entry := s.Enrollments[id]
		change(&entry)
		s.Enrollments[id] = entry
	}
	observation := func(s *Snapshot, id string, change func(*Observation)) {
		entry := s.Observations[id]
		change(&entry)
		s.Observations[id] = entry
	}
	cases := []struct {
		name, check string
		change      func(*Snapshot)
		// baseline is a change of the snapshot the run began with.
		baseline func(*Snapshot)
		written  map[string]string
		// fault is part of the reading of a failure; empty means the check passes.
		fault string
	}{
		{name: "nothing changed", check: "enrollments-enrolled"},
		{name: "a Pending enrollment that was Pending", check: "enrollments-enrolled", change: func(s *Snapshot) {
			enrollment(s, "office/desk-2", func(e *Enrollment) { e.Phase = "Rejected" })
		}},
		{name: "an Enrolled one turned Pending", check: "enrollments-enrolled", fault: "office/s1u-01 was Enrolled, is Pending", change: func(s *Snapshot) {
			enrollment(s, "office/s1u-01", func(e *Enrollment) { e.Phase = "Pending" })
		}},
		{name: "an Enrolled one is gone", check: "enrollments-enrolled", fault: "cloud/gpu-1 was Enrolled, is absent", change: func(s *Snapshot) {
			delete(s.Enrollments, "cloud/gpu-1")
		}},
		{name: "a new problem", check: "enrollments-enrolled", fault: "office/s1u-01 has a new problem: endpoint unknown", change: func(s *Snapshot) {
			enrollment(s, "office/s1u-01", func(e *Enrollment) { e.Problems = []string{"endpoint unknown"} })
		}},

		{name: "nothing changed", check: "documents-unchanged"},
		{name: "a document changed", check: "documents-unchanged", fault: "office/s1u-01 document bbbbbbbbbbbb -> dddddddddddd", change: func(s *Snapshot) {
			enrollment(s, "office/s1u-01", func(e *Enrollment) { e.ConfigHash = "dddddddddddddddd" })
		}},
		{name: "a document that was none comes back", check: "documents-unchanged", baseline: func(s *Snapshot) {
			enrollment(s, "office/s1u-01", func(e *Enrollment) { e.ConfigHash = "" })
		}},

		{name: "a Silent link that was Silent", check: "links-connected", change: func(s *Snapshot) {
			enrollment(s, "office/desk-2", func(e *Enrollment) { e.Links = map[string]string{"cloud/gpu-1": "Unanswered"} })
		}},
		{name: "a Connected link turned Silent", check: "links-connected", fault: "office/s1u-01 -> cloud/gpu-1 was Connected, is Silent", change: func(s *Snapshot) {
			enrollment(s, "office/s1u-01", func(e *Enrollment) { e.Links = map[string]string{"cloud/gpu-1": "Silent"} })
		}},
		{name: "a Connected link turned Unknown", check: "links-connected", fault: "was Connected, is Unknown", change: func(s *Snapshot) {
			enrollment(s, "office/s1u-01", func(e *Enrollment) { e.Links = map[string]string{"cloud/gpu-1": "Unknown"} })
		}},
		{name: "a new link that is Unanswered", check: "links-connected", fault: "office/s1u-01 -> cloud/gpu-2 was absent, is Unanswered", change: func(s *Snapshot) {
			enrollment(s, "office/s1u-01", func(e *Enrollment) {
				e.Links = map[string]string{"cloud/gpu-1": "Connected", "cloud/gpu-2": "Unanswered"}
			})
		}},

		{name: "not FullyConnected as before", check: "network-connected"},
		{name: "a pair fewer", check: "network-connected", change: func(s *Snapshot) { s.SiteNetwork.Unreachable = nil }},
		{name: "no longer Ready", check: "network-connected", fault: "SiteNetwork is no longer Ready", change: func(s *Snapshot) { s.SiteNetwork.Ready = false }},
		{name: "a pair more", check: "network-connected", fault: "no path office/s1u-01 -> cloud/gpu-1", change: func(s *Snapshot) {
			s.SiteNetwork.Unreachable = append(s.SiteNetwork.Unreachable, "office/s1u-01 -> cloud/gpu-1")
		}},

		{name: "Downgraded as before", check: "clusters-online"},
		{name: "Downgraded turned Online", check: "clusters-online", change: func(s *Snapshot) { s.Clusters["office"] = "Online" }},
		{name: "Online turned Downgraded", check: "clusters-online", fault: "cluster cloud was Online, is Downgraded", change: func(s *Snapshot) { s.Clusters["cloud"] = "Downgraded" }},
		{name: "Downgraded turned Offline", check: "clusters-online", fault: "cluster office was Downgraded, is Offline", change: func(s *Snapshot) { s.Clusters["office"] = "Offline" }},

		{name: "an Offline node that was Offline", check: "nodes-ready"},
		{name: "an Online node turned Offline", check: "nodes-ready", fault: "node office/s1u-01 was Online, is Offline", change: func(s *Snapshot) { s.Nodes["office/s1u-01"] = "Offline" }},

		{name: "an Installing addon that was Installing", check: "addons-ready"},
		{name: "a Ready addon turned Installing", check: "addons-ready", fault: "addon cloud/station was Ready, is Installing", change: func(s *Snapshot) {
			s.Addons["cloud/station"] = Addon{Phase: "Installing", Version: "0.4.0"}
		}},
		{name: "a version changed", check: "addons-ready", fault: "addon cloud/station version 0.4.0 -> 0.5.0", change: func(s *Snapshot) {
			s.Addons["cloud/station"] = Addon{Phase: "Ready", Version: "0.5.0"}
		}},
		{name: "a version the run wrote, at that version", check: "addons-ready", written: map[string]string{"cloud/station": "0.5.0"}, change: func(s *Snapshot) {
			s.Addons["cloud/station"] = Addon{Phase: "Ready", Version: "0.5.0"}
		}},
		{name: "a version the run wrote, at another version", check: "addons-ready", fault: "addon cloud/station version 0.4.0 -> 0.6.0", written: map[string]string{"cloud/station": "0.5.0"}, change: func(s *Snapshot) {
			s.Addons["cloud/station"] = Addon{Phase: "Ready", Version: "0.6.0"}
		}},

		{name: "a stale node that was stale", check: "observations-fresh"},
		{name: "a fresh node turned stale", check: "observations-fresh", fault: "office/s1u-01 last reported 5m0s ago", change: func(s *Snapshot) {
			s.Observations["office/s1u-01"] = Observation{ReportedAt: taken.Add(-5 * time.Minute)}
		}},
		{name: "a fresh node without an observation", check: "observations-fresh", fault: "cloud/gpu-1 has no observation", change: func(s *Snapshot) {
			delete(s.Observations, "cloud/gpu-1")
		}},

		{name: "a full disk that was full", check: "nodes-unpressured"},
		{name: "a pressure gone", check: "nodes-unpressured", change: func(s *Snapshot) { observation(s, "office/s1u-01", func(o *Observation) { o.Pressures = []string{} }) }},
		{name: "a new pressure", check: "nodes-unpressured", fault: "cloud/gpu-1 is under DiskPressure, was not", change: func(s *Snapshot) {
			observation(s, "cloud/gpu-1", func(o *Observation) { o.Pressures = []string{"DiskPressure"} })
		}},
		{name: "another pressure than the old one", check: "nodes-unpressured", fault: "office/s1u-01 is under MemoryPressure, was not", change: func(s *Snapshot) {
			observation(s, "office/s1u-01", func(o *Observation) { o.Pressures = []string{"DiskPressure", "MemoryPressure"} })
		}},
		{name: "a pressure in a report that is not fresh", check: "nodes-unpressured", change: func(s *Snapshot) {
			observation(s, "office/desk-2", func(o *Observation) { o.Pressures = []string{"PIDPressure"} })
		}},
		{name: "a report without pressures", check: "nodes-unpressured", change: func(s *Snapshot) {
			observation(s, "cloud/gpu-1", func(o *Observation) { o.Pressures = nil })
		}},
	}
	covered := map[string]bool{}
	for _, c := range cases {
		covered[c.check] = true
		t.Run(c.check+"/"+c.name, func(t *testing.T) {
			current := site()
			if c.change != nil {
				c.change(&current)
			}
			began := site()
			if c.baseline != nil {
				c.baseline(&began)
			}
			result := Checks[c.check](began, current, c.written)
			if result.OK != (c.fault == "") || !strings.Contains(result.Reading, c.fault) {
				t.Fatalf("ok=%t reading=%q, want fault %q", result.OK, result.Reading, c.fault)
			}
		})
	}
	for name := range Checks {
		if !covered[name] {
			t.Errorf("check %s has no case", name)
		}
	}
}

func TestNodesUnpressuredSaysWhatItDidNotRead(t *testing.T) {
	// The baseline of the batch that brings pressures: no report has them.
	baseline := site()
	for id, observation := range baseline.Observations {
		observation.Pressures = nil
		baseline.Observations[id] = observation
	}
	if result := nodesUnpressured(baseline, baseline, nil); !result.OK || !strings.Contains(result.Reading, "0 nodes under no pressure they were not under before; 3 not read") {
		t.Fatalf("nothing reported: %+v", result)
	}
	if result := nodesUnpressured(baseline, site(), nil); result.OK || result.Reading != "office/s1u-01 is under DiskPressure; the baseline has no reading of it" {
		t.Fatalf("a pressure the baseline could not have: %+v", result)
	}
	if result := nodesUnpressured(site(), site(), nil); !result.OK || !strings.Contains(result.Reading, "2 nodes under no pressure they were not under before; 1 not read") {
		t.Fatalf("a stale node: %+v", result)
	}
}

func TestGatewaysUnderstand(t *testing.T) {
	for name, c := range map[string]struct {
		level  int32
		change func(*Snapshot)
		fault  string
	}{
		"every gateway at the level, the node that is off by its last report": {level: 2},
		"a higher level than any has":                                         {level: 3, fault: "cloud/gpu-1 gateway understands level 2; office/desk-2 gateway understands level 2; office/s1u-01 gateway understands level 2"},
		"one gateway behind": {level: 2, fault: "office/s1u-01 gateway understands level 1", change: func(s *Snapshot) {
			s.Observations["office/s1u-01"] = Observation{Fresh: true, GatewayLevel: 1}
		}},
		"a gateway that is not reconciling": {level: 1, fault: "office/s1u-01 gateway states no level", change: func(s *Snapshot) {
			s.Observations["office/s1u-01"] = Observation{Fresh: true}
		}},
		"a member that never reported": {level: 1, fault: "cloud/gpu-1 gateway states no level", change: func(s *Snapshot) {
			delete(s.Observations, "cloud/gpu-1")
		}},
		"a member without a document": {level: 2, change: func(s *Snapshot) {
			s.Enrollments["office/desk-3"] = Enrollment{Phase: "Pending"}
		}},
	} {
		current := site()
		if c.change != nil {
			c.change(&current)
		}
		result := Condition{Check: "gateways-understand", Level: c.level}.Verify(Snapshot{}, current, nil)
		if result.OK != (c.fault == "") || c.fault != "" && result.Reading != c.fault {
			t.Errorf("%s: ok=%t reading=%q", name, result.OK, result.Reading)
		}
	}
	if result := gatewaysUnderstand(site(), 2); result.Reading != "the gateways of all 3 members with a document understand level 2" {
		t.Errorf("reading %q", result.Reading)
	}
}
