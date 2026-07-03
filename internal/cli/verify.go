package cli

import (
	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

// buildVerifiers turns an environment's verify config into a composite verifier, using runner
// for host-side probes. compose-ps is wired to the executor's kind-aware ComposeTarget, so it
// checks the right project (current/.env for symlink-release, the idle slot for blue-green). It
// returns nil when there are no probes.
func buildVerifiers(probes []config.VerifyProbe, runner verify.Runner, exec executor.Executor) verify.Verifier {
	if len(probes) == 0 {
		return nil
	}
	comp := make(verify.Composite, 0, len(probes))
	for _, p := range probes {
		switch p.Kind {
		case "http":
			comp = append(comp, verify.HTTPVerifier{URL: p.URL, ExpectStatus: p.ExpectStatus})
		case "command":
			comp = append(comp, verify.CommandVerifier{Runner: runner, Command: p.Command})
		case "compose-ps":
			if cv, ok := exec.(verify.ComposeVerifiable); ok {
				comp = append(comp, verify.ComposePSVerifier{Runner: runner, Target: cv.ComposeTarget})
			}
		}
	}
	return comp
}
