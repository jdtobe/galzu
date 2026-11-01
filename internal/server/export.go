package server

import (
	"fmt"
	"net/http"
	"strings"
)

// exportSession renders the durable record as Markdown: what was asked, who
// spoke, what was found, what was voted, and what was planned. It is the same
// data the UI shows, in a form a human can keep.
func (v *server) exportSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	s, agents, err := v.s.GetSession(ctx, id)
	if err != nil {
		failure(w, err)
		return
	}
	messages, err := v.s.Messages(ctx, id, 0)
	if err != nil {
		failure(w, err)
		return
	}
	plans, err := v.s.Plans(ctx, id)
	if err != nil {
		failure(w, err)
		return
	}
	proposals, err := v.s.Proposals(ctx, id)
	if err != nil {
		failure(w, err)
		return
	}
	findings, err := v.s.Findings(ctx, id, 0)
	if err != nil {
		failure(w, err)
		return
	}
	names := map[string]string{}
	for _, agent := range agents {
		names[agent.ID] = agent.Name
	}
	name := func(agentID string) string {
		if agentID == "" {
			return "Human"
		}
		if value := names[agentID]; value != "" {
			return value
		}
		return agentID
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", strings.TrimSpace(s.Name))
	if s.Status != "" {
		fmt.Fprintf(&b, "**Status:** %s  \n", s.Status)
	}
	fmt.Fprintf(&b, "**Goal:** %s\n\n", s.Goal)
	if s.Brief != "" {
		fmt.Fprintf(&b, "**Brief:** %s\n\n", s.Brief)
	}

	b.WriteString("## Agents\n\n")
	for _, agent := range agents {
		fmt.Fprintf(&b, "- **%s** · %s · `%s` · %s\n", agent.Name, agent.Role, agent.Model, agent.Status)
	}
	b.WriteString("\n")

	if len(plans) > 0 {
		b.WriteString("## Plans\n\n")
		for _, plan := range plans {
			fmt.Fprintf(&b, "### Plan v%d · %s\n\n", plan.Version, plan.Status)
			if plan.ProposalID > 0 {
				fmt.Fprintf(&b, "Decision: proposal #%d passed.\n\n", plan.ProposalID)
			}
			b.WriteString(plan.Body)
			b.WriteString("\n\n")
			if plan.Note != "" {
				fmt.Fprintf(&b, "> Human note: %s\n\n", plan.Note)
			}
		}
	}

	if len(proposals) > 0 {
		b.WriteString("## Questions and ballots\n\n")
		for _, proposal := range proposals {
			fmt.Fprintf(&b, "### Question #%d · %s", proposal.ID, proposal.Status)
			if proposal.Kind != "" && proposal.Kind != "vote" {
				fmt.Fprintf(&b, " · %s", proposal.Kind)
			}
			b.WriteString("\n\n")
			b.WriteString(proposal.Body)
			b.WriteString("\n\n")
			ballots, err := v.s.Ballots(ctx, proposal.ID)
			if err != nil {
				failure(w, err)
				return
			}
			if len(ballots) == 0 {
				b.WriteString("_No ballots cast._\n\n")
			}
			for _, ballot := range ballots {
				fmt.Fprintf(&b, "- %s: **%s**", name(ballot.AgentID), ballot.Choice)
				if ballot.Text != "" {
					fmt.Fprintf(&b, " — %s", ballot.Text)
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	if len(findings) > 0 {
		b.WriteString("## Findings\n\n")
		for _, finding := range findings {
			fmt.Fprintf(&b, "### Finding #%d · %s\n\n%s\n\n", finding.ID, finding.Status, finding.Claim)
			for _, evidence := range finding.Evidence {
				fmt.Fprintf(&b, "- **%s** · %s: %s\n", evidence.Stance, name(evidence.AuthorID), evidence.Body)
			}
			if len(finding.Evidence) > 0 {
				b.WriteString("\n")
			}
			if len(finding.Positions) > 0 {
				b.WriteString("Positions: ")
				parts := make([]string, 0, len(finding.Positions))
				for _, position := range finding.Positions {
					parts = append(parts, fmt.Sprintf("%s %s", name(position.AgentID), position.Choice))
				}
				b.WriteString(strings.Join(parts, " · "))
				b.WriteString("\n\n")
			}
			for _, review := range finding.Reviews {
				fmt.Fprintf(&b, "Review requested from %s: %s (%s)\n\n", name(review.ReviewerID), review.Request, review.Status)
			}
			if len(finding.Revisions) > 1 {
				fmt.Fprintf(&b, "%d revisions; earlier claims are kept in the record.\n\n", len(finding.Revisions)-1)
			}
		}
	}

	b.WriteString("## Conversation\n\n")
	for _, message := range messages {
		kind := ""
		if message.Kind != "" {
			kind = " · " + message.Kind
		}
		if message.RecipientID != "" {
			fmt.Fprintf(&b, "**%s → %s**%s\n\n%s\n\n", name(message.AgentID), name(message.RecipientID), kind, message.Body)
			continue
		}
		fmt.Fprintf(&b, "**%s**%s\n\n%s\n\n", name(message.AgentID), kind, message.Body)
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="galzu-%s.md"`, id))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}
