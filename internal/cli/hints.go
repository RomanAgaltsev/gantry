package cli

import "fmt"

// deployFailureHint returns operator guidance for a promote/rollback that committed a new
// pin set but then failed to deploy it. committed is the SHA of that commit; an empty
// committed (the deploy never got that far) yields no hint.
func deployFailureHint(env, committed string) string {
	if committed == "" {
		return ""
	}
	return fmt.Sprintf("note: %s pin committed at %.7s but the deploy failed; "+
		"run `gantry deploy --env %s` to retry", env, committed, env)
}

// promoteDAGWarning returns a warning when a promotion runs against an edge other than the
// one the target environment's config declares (its promote_from). It is advisory only —
// explicit --from/--to edges are allowed — so an unset or matching promote_from is silent.
func promoteDAGWarning(toEnv, configuredFrom, actualFrom string) string {
	if configuredFrom == "" || configuredFrom == actualFrom {
		return ""
	}
	return fmt.Sprintf("warning: %s.promote_from is %q, but promoting from %q",
		toEnv, configuredFrom, actualFrom)
}

// autoRollbackNote returns the operator note for a command whose deploy failed verification
// and was auto-rolled-back. Empty when no rollback occurred.
func autoRollbackNote(env, rolledBackTo string) string {
	if rolledBackTo == "" {
		return ""
	}
	return fmt.Sprintf("verify failed for %s; rolled back to %.7s", env, rolledBackTo)
}