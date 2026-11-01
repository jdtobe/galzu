package governance

import "testing"

func TestDecideNeedsEveryEligibleBallot(t *testing.T) {
	eligible := []string{"a", "b", "c"}
	v := Decide(eligible, []Ballot{{AgentID: "a", Choice: Yes}, {AgentID: "b", Choice: Yes}}, false)
	if v.Decided {
		t.Fatalf("closed early: %+v", v)
	}
	if v.Reason != "waiting for 1 of 3" {
		t.Fatalf("reason = %q", v.Reason)
	}
}

func TestDecideExpiryClosesWithPartialParticipation(t *testing.T) {
	eligible := []string{"a", "b", "c"}
	v := Decide(eligible, []Ballot{{AgentID: "a", Choice: Yes}}, true)
	if !v.Decided || !v.Passed {
		t.Fatalf("expired vote = %+v, want decided and passed", v)
	}
	if v.Cast != 1 || v.Eligible != 3 {
		t.Fatalf("cast %d of %d", v.Cast, v.Eligible)
	}
}

// Abstention is absence, never a no: a vote with no ballots closes unpassed.
func TestDecideWithNoBallotsFails(t *testing.T) {
	v := Decide([]string{"a", "b"}, nil, true)
	if !v.Decided || v.Passed {
		t.Fatalf("empty vote = %+v", v)
	}
	if v.Reason != "closed with no ballots" {
		t.Fatalf("reason = %q", v.Reason)
	}
}

func TestDecideTiesFail(t *testing.T) {
	v := Decide([]string{"a", "b"}, []Ballot{{AgentID: "a", Choice: Yes}, {AgentID: "b", Choice: No}}, false)
	if !v.Decided || v.Passed {
		t.Fatalf("tie = %+v, want decided and not passed", v)
	}
}

func TestDecideMajorityPasses(t *testing.T) {
	v := Decide([]string{"a", "b", "c"}, []Ballot{{AgentID: "a", Choice: Yes}, {AgentID: "b", Choice: Yes}, {AgentID: "c", Choice: No}}, false)
	if !v.Decided || !v.Passed || v.Yes != 2 || v.No != 1 {
		t.Fatalf("%+v", v)
	}
}

// Ballots from agents who are no longer eligible do not count.
func TestDecideIgnoresIneligibleBallots(t *testing.T) {
	v := Decide([]string{"a"}, []Ballot{{AgentID: "a", Choice: No}, {AgentID: "ghost", Choice: Yes}}, false)
	if !v.Decided || v.Passed || v.Yes != 0 {
		t.Fatalf("%+v", v)
	}
}

func TestNormaliseChoice(t *testing.T) {
	for in, want := range map[string]string{
		"yes": Yes, " YES ": Yes, "Approve": Yes, "no": No, "against": No, "No.": "",
	} {
		got, ok := NormaliseChoice(in)
		if want == "" {
			if ok {
				t.Fatalf("%q accepted as %q", in, got)
			}
			continue
		}
		if !ok || got != want {
			t.Fatalf("%q -> %q %v, want %q", in, got, ok, want)
		}
	}
}

func TestElectionChoosesTheMostBallots(t *testing.T) {
	e := DecideElection([]string{"a", "b", "c"}, []string{"a", "b", "c"},
		[]Ballot{{AgentID: "a", Choice: "b"}, {AgentID: "b", Choice: "b"}, {AgentID: "c", Choice: "c"}}, false)
	if !e.Decided || e.Winner != "b" || e.Counts["b"] != 2 {
		t.Fatalf("%+v", e)
	}
}

func TestElectionTieHasNoWinner(t *testing.T) {
	e := DecideElection([]string{"a", "b"}, []string{"a", "b"},
		[]Ballot{{AgentID: "a", Choice: "a"}, {AgentID: "b", Choice: "b"}}, false)
	if !e.Decided || !e.Tie || e.Winner != "" {
		t.Fatalf("%+v", e)
	}
}

// A ballot for someone who is not standing is ignored, not treated as a vote.
func TestElectionIgnoresIneligibleCandidates(t *testing.T) {
	e := DecideElection([]string{"a", "b"}, []string{"a", "b"},
		[]Ballot{{AgentID: "a", Choice: "ghost"}, {AgentID: "b", Choice: "a"}}, false)
	if !e.Decided || e.Winner != "a" || e.Cast != 2 {
		t.Fatalf("%+v", e)
	}
}
