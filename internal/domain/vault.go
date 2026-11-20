package domain

// Vault is the decrypted set of entries. Entries arrive in a later slice; for
// now a vault is always empty.
type Vault struct{}

// NewVault returns an empty vault.
func NewVault() Vault { return Vault{} }

// Len is the number of entries in the vault.
func (v Vault) Len() int { return 0 }
