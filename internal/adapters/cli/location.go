package cli

import (
	"fmt"
	"path/filepath"
)

const (
	envVault         = "SAVE_PWD_VAULT"
	defaultVaultDir  = "save_pwd"
	defaultVaultFile = "vault.db"
)

// vaultLocator resolves the vault path as --vault > SAVE_PWD_VAULT > the user
// config dir default. Only vault commands use it, so generate never touches
// the environment or the config dir.
type vaultLocator struct {
	opts      *rootOptions
	lookupEnv func(key string) (string, bool)
	configDir func() (string, error)
}

func (l vaultLocator) resolve() (string, error) {
	if l.opts.vaultPath != "" {
		return l.opts.vaultPath, nil
	}
	// An empty SAVE_PWD_VAULT counts as unset, so `SAVE_PWD_VAULT= save_pwd`
	// falls back to the default instead of failing on an empty path.
	if p, ok := l.lookupEnv(envVault); ok && p != "" {
		return p, nil
	}
	dir, err := l.configDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine the default vault location (%w); use --vault PATH or set %s", err, envVault)
	}
	return filepath.Join(dir, defaultVaultDir, defaultVaultFile), nil
}
