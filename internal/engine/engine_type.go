package engine

import (
	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
)

// Engine bundles the long-lived collaborators every verb needs — the config, the forge
// client, the pin store, and the deploy ledger — so verbs are methods taking only their
// per-call arguments (the executor and verifier, which are rebuilt per environment). This
// replaces threading (cfg, f, store, led) through every verb and duplicating them in
// cli.deps and daemon.Deps (review §2.2-A).
type Engine struct {
	Cfg    *config.Config
	Forge  forge.Forge
	Store  PinStore
	Ledger ledger.Ledger
}

// New builds an Engine from its collaborators.
func New(cfg *config.Config, f forge.Forge, store PinStore, led ledger.Ledger) *Engine {
	return &Engine{Cfg: cfg, Forge: f, Store: store, Ledger: led}
}
