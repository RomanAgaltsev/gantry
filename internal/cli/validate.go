package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/engine"
)

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the config: schema, resolvable secrets, and pin/config coherence",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := cmd.Flags().GetString("config")
			if err != nil {
				return err
			}
			cfg, err := config.Load(path) // schema validation (already thorough)
			if err != nil {
				return err
			}
			res := config.DefaultResolver()
			resolveVaultDefaults(cmd.Context(), &res, cfg)
			if err := resolveAllSecrets(cmd.Context(), res, cfg); err != nil {
				return err
			}
			store, err := engine.NewGitStore(filepath.Dir(path),
				object.Signature{Name: cfg.Git.AuthorName, Email: cfg.Git.AuthorEmail})
			if err != nil {
				return err
			}
			for _, env := range cfg.Environments {
				current, err := store.Read(cmd.Context(), env.PinFile)
				if err != nil {
					return err
				}
				for _, k := range engine.Orphans(cfg, current) {
					cmd.PrintErrf("warning: %s: orphan pin %q (in pin file, not in config)\n", env.Name, k)
				}
				for _, k := range engine.MissingKeys(cfg, current) {
					cmd.PrintErrf("warning: %s: missing pin %q (in config, not in pin file)\n", env.Name, k)
				}
			}
			cmd.Println("config valid")
			return nil
		},
	}
	return cmd
}

// resolveAllSecrets resolves every SecretRef the config references, returning the first error
// (e.g. an unset ${env:…} or an unreadable ${file:…}). Values are discarded — validate never
// prints a secret. The ambient secrets.vault.address/token are deliberately excluded: they
// default to ${env:VAULT_ADDR}/${env:VAULT_TOKEN} and are resolved best-effort at use, so a
// config that never uses a ${vault:…} ref must still validate with those env vars unset.
func resolveAllSecrets(ctx context.Context, res config.SecretResolver, cfg *config.Config) error {
	var refs []config.SecretRef
	refs = append(refs, cfg.Forge.Token)
	for _, conn := range cfg.Connections {
		if conn.SSH != nil {
			refs = append(refs, conn.SSH.Key, conn.SSH.KnownHosts)
		}
	}
	for _, reg := range cfg.Registries {
		refs = append(refs, reg.User, reg.Password)
	}
	for _, ch := range cfg.Notifications {
		refs = append(refs, ch.URL, ch.ChatID, ch.SMTP.Password)
	}
	if cfg.Daemon.Doorbell.Enabled {
		refs = append(refs, cfg.Daemon.Doorbell.Secret)
	}
	for _, r := range refs {
		if r.Raw == "" {
			continue
		}
		if _, err := res.Resolve(ctx, r); err != nil {
			return fmt.Errorf("secret %q: %w", r.Raw, err)
		}
	}
	return nil
}
