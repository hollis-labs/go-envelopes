package envelopes

import (
	"fmt"
	"sort"
	"strings"
)

// TypeSupport is a consumer's explicit statement of which envelope types it
// handles, declared BY NAME.
//
// # WHY THIS EXISTS, AND WHY IT IS NOT A CATALOG SPLIT
//
// A host that adopts the core catalog wholesale imports every type in it,
// including types it has no business rendering. Tangent, for example, is not an
// agent launcher or a session manager, so `subagent-spawn-approval` and
// `chat-loop-terminated` are boundary violations for it.
//
// The tempting fix is to partition the catalog so a consumer can take one half.
// That fix was investigated (CW-20260910-0122) and rejected on the evidence:
//
//   - There is no wire-kind versus composition split to make. No core schema
//     references another type's schema — every $ref is a local #/$defs pointer
//     — and types once thought to be composition-only (`list-card`,
//     `confirmation-card`) are emitted standalone as whole interactions, which
//     is what their `data_source` pointers exist to support. All core types are
//     wire kinds.
//   - The distinction that does exist is EMISSION AUTHORITY: who may emit a
//     type — an agent, a host decision flow, or the runtime. But that is host
//     policy, not wire. Nanite says so in its own source: its card_show
//     allow-list is annotated "It is host presentation policy; schema ownership
//     stays with go-envelopes." Encoding one host's answer here would repeat
//     the mistake of the `component` field removed in v0.5.0 — a shared
//     contract asserting a single host's local arrangement.
//
// So the library does not decide which types a consumer supports. It gives the
// consumer a way to state its own answer and to be told when that answer has
// gone stale.
//
// # EXCLUSION BY NAME, NOT BY OMISSION
//
// Every type in the catalog must be claimed: either supported, or excluded with
// a reason. A type that is neither is reported as unclaimed, which is an error.
//
// This is the whole point. Excluding a type by leaving it out of a list is
// indistinguishable from never having heard of it, so the next time a consumer
// regenerates against a newer catalog, a type that was deliberately rejected
// comes back silently. Requiring a named exclusion with a reason makes adopting
// a new type a decision someone has to write down, and makes an old decision
// survive the regeneration that would otherwise erase it.
type TypeSupport struct {
	// Supports lists the type names this consumer renders or handles.
	Supports []string

	// Excludes maps a type name to the reason it is deliberately not
	// supported. The reason is required: an exclusion whose rationale is
	// missing is one a later reader deletes as dead configuration.
	//
	// e.g. "subagent-spawn-approval": "ADR 0005 — Tangent is not a session manager"
	Excludes map[string]string
}

// SupportGap describes the ways a TypeSupport declaration fails to account for
// the catalog it was checked against. It implements error so a consumer can
// fail a build or a test on it directly.
type SupportGap struct {
	// Unclaimed are catalog types the declaration neither supports nor
	// excludes — new types that arrived since it was written.
	Unclaimed []string

	// Unknown are names the declaration mentions that the catalog does not
	// contain: a typo, or a type that has since been removed.
	Unknown []string

	// Conflicting are names listed as both supported and excluded.
	Conflicting []string
}

// Error implements error. The message names every gap, because the caller's
// next action is to resolve each one individually.
func (g *SupportGap) Error() string {
	var parts []string
	if len(g.Unclaimed) > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d catalog type(s) neither supported nor excluded: %s (support them, or exclude them by name with a reason)",
			len(g.Unclaimed), strings.Join(g.Unclaimed, ", ")))
	}
	if len(g.Unknown) > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d declared name(s) not in the catalog: %s",
			len(g.Unknown), strings.Join(g.Unknown, ", ")))
	}
	if len(g.Conflicting) > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d name(s) both supported and excluded: %s",
			len(g.Conflicting), strings.Join(g.Conflicting, ", ")))
	}
	return "envelopes: incomplete type support: " + strings.Join(parts, "; ")
}

// CheckSupport verifies that support accounts for every registered type, and
// that every name it mentions exists.
//
// Returns nil when the declaration is complete. Otherwise returns a *SupportGap
// naming what is missing; the returned error is non-nil as an error interface
// value only when there is a real gap, so `if err := r.CheckSupport(s); err !=
// nil` behaves as expected.
//
// The intended use is a consumer-side test, so that upgrading the library turns
// a newly added envelope type into a failing build rather than a silent import:
//
//	func TestEnvelopeSupportIsCurrent(t *testing.T) {
//	    registry, err := envelopes.LoadCore(context.Background())
//	    if err != nil { t.Fatal(err) }
//	    if err := registry.CheckSupport(envelopes.TypeSupport{
//	        Supports: hostRenderedTypes,
//	        Excludes: map[string]string{
//	            "subagent-spawn-approval": "ADR 0005 — not a session manager",
//	            "chat-loop-terminated":    "ADR 0005 — not an agent launcher",
//	        },
//	    }); err != nil {
//	        t.Fatal(err)
//	    }
//	}
func (r *Registry) CheckSupport(support TypeSupport) error {
	supported := make(map[string]bool, len(support.Supports))
	for _, name := range support.Supports {
		supported[name] = true
	}

	registered := make(map[string]bool)
	for _, spec := range r.All() {
		registered[spec.Name] = true
	}

	gap := &SupportGap{}
	for name := range registered {
		_, excluded := support.Excludes[name]
		if !supported[name] && !excluded {
			gap.Unclaimed = append(gap.Unclaimed, name)
		}
	}
	for name := range supported {
		if _, alsoExcluded := support.Excludes[name]; alsoExcluded {
			gap.Conflicting = append(gap.Conflicting, name)
		}
		if !registered[name] {
			gap.Unknown = append(gap.Unknown, name)
		}
	}
	for name := range support.Excludes {
		if !registered[name] {
			gap.Unknown = append(gap.Unknown, name)
		}
	}

	sort.Strings(gap.Unclaimed)
	sort.Strings(gap.Unknown)
	sort.Strings(gap.Conflicting)
	gap.Unknown = dedupeSorted(gap.Unknown)

	if len(gap.Unclaimed) == 0 && len(gap.Unknown) == 0 && len(gap.Conflicting) == 0 {
		return nil
	}
	return gap
}

// SupportedTypes returns the type names support accounts for as supported,
// filtered to those actually registered and sorted. It is the set a consumer
// should build its own renderer table from — deriving that table from this
// rather than from r.All() is what keeps an unclaimed type out of a host's
// dispatch map even before CheckSupport is consulted.
func (r *Registry) SupportedTypes(support TypeSupport) []string {
	registered := make(map[string]bool)
	for _, spec := range r.All() {
		registered[spec.Name] = true
	}
	out := make([]string, 0, len(support.Supports))
	for _, name := range support.Supports {
		if registered[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return dedupeSorted(out)
}

func dedupeSorted(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for _, value := range in[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
