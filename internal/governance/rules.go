// Package governance holds the voting rules: who may vote, when a vote closes,
// and what counts as agreement.
//
// The rules are pure so they can be read and tested without a store, a model or
// a runner. Spec: docs/superpowers/specs/2026-11-02-governance.md
package governance

import "fmt"

// Ballot is one agent's current position on one proposal. A changed ballot
// replaces the previous one; the store keeps the latest per agent.
type Ballot struct {
	AgentID string
	Choice  string // "yes" or "no"
	Text    string
}

const (
	Yes = "yes"
	No  = "no"
)

// Verdict is the outcome of a tally. Cast counts distinct eligible voters who
// have voted; it is not the number of ballots ever written.
type Verdict struct {
	Decided  bool
	Passed   bool
	Yes      int
	No       int
	Cast     int
	Eligible int
	Reason   string
}

// Decide tallies a proposal. A vote closes when every eligible agent has voted,
// or when it expires; on expiry, abstention is recorded as absence and never as
// a no. A closed vote passes only on a strict majority of ballots cast, so ties
// fail.
func Decide(eligible []string, ballots []Ballot, expired bool) Verdict {
	seen := make(map[string]Ballot, len(ballots))
	for _, b := range ballots {
		seen[b.AgentID] = b
	}
	verdict := Verdict{Eligible: len(eligible)}
	for _, id := range eligible {
		choice, ok := seen[id]
		if !ok {
			continue
		}
		verdict.Cast++
		switch choice.Choice {
		case Yes:
			verdict.Yes++
		case No:
			verdict.No++
		}
	}
	switch {
	case verdict.Eligible == 0:
		verdict.Decided, verdict.Passed, verdict.Reason = true, false, "no eligible voters"
	case verdict.Cast >= verdict.Eligible:
		verdict.Decided = true
	case expired:
		verdict.Decided = true
	}
	if !verdict.Decided {
		verdict.Reason = fmt.Sprintf("waiting for %d of %d", verdict.Eligible-verdict.Cast, verdict.Eligible)
		return verdict
	}
	if verdict.Cast == 0 {
		verdict.Reason = "closed with no ballots"
		return verdict
	}
	verdict.Passed = verdict.Yes > verdict.No
	if verdict.Passed {
		verdict.Reason = "passed"
	} else {
		verdict.Reason = "not passed"
	}
	return verdict
}

// NormaliseChoice accepts the small spellings a model actually emits and says
// no to everything else.
func NormaliseChoice(text string) (string, bool) {
	switch lower := trimLower(text); lower {
	case "yes", "y", "true", "for", "approve", "agree":
		return Yes, true
	case "no", "n", "false", "against", "reject", "disagree":
		return No, true
	}
	return "", false
}

func trimLower(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	out := make([]byte, 0, end-start)
	for _, c := range []byte(s[start:end]) {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}

// Election is the outcome of choosing a leader. Ballots name a candidate in
// Choice, so the same table holds both kinds; a ballot for someone ineligible is
// ignored rather than counted as abstention.
type Election struct {
	Decided  bool
	Winner   string
	Tie      bool
	Counts   map[string]int
	Cast     int
	Eligible int
	Reason   string
}

// DecideElection chooses the candidate with the most ballots. It closes on the
// same terms as a vote; a tie leaves the herd without a winner, because picking
// one by any other rule would be the harness deciding who leads.
func DecideElection(eligible, candidates []string, ballots []Ballot, expired bool) Election {
	out := Election{Eligible: len(eligible), Counts: map[string]int{}}
	allowed := make(map[string]bool, len(candidates))
	for _, id := range candidates {
		allowed[id] = true
	}
	seen := map[string]Ballot{}
	for _, b := range ballots {
		seen[b.AgentID] = b
	}
	for _, id := range eligible {
		ballot, ok := seen[id]
		if !ok {
			continue
		}
		// A ballot for someone who is not standing still counts as having
		// voted: the agent participated, even if the choice was invalid.
		out.Cast++
		if allowed[ballot.Choice] {
			out.Counts[ballot.Choice]++
		}
	}
	switch {
	case out.Eligible == 0:
		out.Decided, out.Reason = true, "no eligible voters"
		return out
	case out.Cast >= out.Eligible, expired:
		out.Decided = true
	}
	if !out.Decided {
		out.Reason = fmt.Sprintf("waiting for %d of %d", out.Eligible-out.Cast, out.Eligible)
		return out
	}
	best, bestCount, tied := "", 0, false
	for _, id := range candidates {
		count := out.Counts[id]
		switch {
		case count > bestCount:
			best, bestCount, tied = id, count, false
		case count == bestCount && count > 0:
			tied = true
		}
	}
	switch {
	case bestCount == 0:
		out.Reason = "closed with no ballots"
	case tied:
		out.Tie, out.Reason = true, "tied"
	default:
		out.Winner, out.Reason = best, "elected"
	}
	return out
}
