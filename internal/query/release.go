package query

import (
	"context"

	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/db/ent"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/platform/mask"
)

// releaseActor is who a result is about to be shown to.
//
// A share reader is expressed as the zero value: no id, no role, therefore no
// grant. That is the honest description of an anonymous holder of a link, and
// it lets the share path enter the same decision as every other reader rather
// than reimplementing the loop beside it.
type releaseActor struct {
	UserID int64
	Role   string
}

// releaseVerdict is what the decision answers.
type releaseVerdict int

const (
	releaseAsIs   releaseVerdict = iota // no rule matched, or the actor may unmask
	releaseMasked                       // rows were rewritten; MaskedFields names what
	releaseRefuse                       // nothing can be shown safely
)

// releaseDecision is the single answer, so callers get a decision rather than a
// checklist.
type releaseDecision struct {
	Verdict      releaseVerdict
	MaskedFields []string
	Reason       error // set when Verdict is releaseRefuse
}

// releaseRows decides what may leave the platform, and rewrites the rows in
// place when the answer is "masked".
//
// This replaced four functions — actorMayUnmask, applyDesensitizationForActor,
// refuseUnmaskableShape and maskingApplies — that answered overlapping parts of
// one question. Two of them were near-identical: both swept Casbin, both loaded
// the rules, both walked mask.MatchRules, and a comment sitting between them
// conceded the duplication was "two chances for an authorization decision to
// drift". Callers paid for it twice per query, because the shape refusal and
// the masking each ran the sweep and the rule read again.
//
// One sweep, one rule read, one shape verdict, in that order:
//
//   - an actor holding unmask sees everything, and nothing else needs asking;
//   - rules that cannot be read are a refusal, never an empty set (invariant 4
//     has no availability exception);
//   - a shape the row masker cannot walk — a driver-native aggregation payload —
//     is refused when any rule matches, because masking it is not possible
//     rather than merely skipped.
func releaseRows(
	ctx context.Context,
	client *ent.Client,
	enforcer ActorEnforcer,
	actor releaseActor,
	datasourceID int64,
	database string,
	tables []string,
	shape driver.ResultShape,
	rows []map[string]interface{},
) (releaseDecision, error) {
	if mayUnmask(ctx, enforcer, actor, datasourceID, tables) {
		return releaseDecision{Verdict: releaseAsIs}, nil
	}

	rules, err := loadMaskRules(ctx, client, datasourceID, database, tables)
	if err != nil {
		return releaseDecision{}, err
	}

	// Which rules actually bear on these targets, computed once and used by both
	// the shape verdict and the rewrite.
	matched := make(map[string][]mask.Rule, len(tables))
	any := false
	for _, table := range tables {
		if r := mask.MatchRules(rules, table); len(r) > 0 {
			matched[table] = r
			any = true
		}
	}

	if !any {
		return releaseDecision{Verdict: releaseAsIs}, nil
	}

	// An aggregation payload is driver-native and arbitrarily nested, so the
	// row-oriented masker cannot reach inside it. Returning one unmasked would
	// turn an aggregation into a way to read protected fields through bucket
	// keys and aggregate values — the export path was missing exactly this
	// condition once.
	if shape == driver.ShapeAggregation {
		return releaseDecision{
			Verdict: releaseRefuse,
			Reason:  ErrAggregationMaskingUnsupported,
		}, nil
	}

	var maskedFields []string
	for _, table := range tables {
		tableRules, ok := matched[table]
		if !ok {
			continue
		}
		// ApplyToMongoRows supports dot-notation paths for nested documents; for
		// flat SQL results it behaves identically to ApplyToRows.
		maskedFields = append(maskedFields, mask.ApplyToMongoRows(rows, tableRules)...)
	}

	if len(maskedFields) == 0 {
		return releaseDecision{Verdict: releaseAsIs}, nil
	}
	return releaseDecision{Verdict: releaseMasked, MaskedFields: maskedFields}, nil
}

// mayUnmask reports whether this actor holds a grant that lets protected fields
// through in the clear.
//
// An enforcer that cannot answer has granted nothing, so masking stays on. That
// is the safe direction, and it is what the two hand-written copies did by
// discarding the error.
func mayUnmask(ctx context.Context, enforcer ActorEnforcer, actor releaseActor, datasourceID int64, tables []string) bool {
	if enforcer == nil {
		return false
	}

	// The canonical action is "unmask"; "desensitize:bypass" is kept so existing
	// installations do not silently lose access.
	granted := func(object string) bool {
		for _, action := range []string{"unmask", "desensitize:bypass"} {
			ok, err := enforcer.EnforceActor(ctx, actor.UserID, actor.Role,
				authz.DatasourceDomain(datasourceID), object, action)
			if err == nil && ok {
				return true
			}
		}
		return false
	}

	for _, table := range tables {
		if table == "" {
			continue
		}
		if granted(table) {
			return true
		}
	}
	return granted("*")
}

// anyRuleProtects reports whether any rule bears on these targets, for callers
// that need the question without the answer's rows.
//
// AI review asks this to tell the reviewer which tables are protected. It used
// to run its own ent query that omitted both the "*" wildcard and the database
// predicate, so a datasource-wide rule masked the rows and reported the table as
// not sensitive.
func anyRuleProtects(ctx context.Context, client *ent.Client, datasourceID int64, database string, tables []string) (bool, error) {
	rules, err := loadMaskRules(ctx, client, datasourceID, database, tables)
	if err != nil {
		return false, err
	}
	for _, table := range tables {
		if len(mask.MatchRules(rules, table)) > 0 {
			return true, nil
		}
	}
	return false, nil
}
