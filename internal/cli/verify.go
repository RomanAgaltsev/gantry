package cli

import (
	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

// buildVerifiers turns an environment's verify config into a composite verifier, using
// runner for host-side probes (compose-ps, command). It returns nil when there are no
// probes, which the engine treats as "no verification" (healthy stays "unknown").
func buildVerifiers(probes []config.VerifyProbe, runner verify.Runner, ex config.ExecutorConfig) verify.Verifier {
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
			comp = append(comp, verify.ComposePSVerifier{
				Runner:       runner,
				ProjectDir:   ex.ProjectDir,
				ComposeFiles: ex.ComposeFiles,
				EnvFile:      ex.EnvFile,
			})
		}
	}
	return comp
}
