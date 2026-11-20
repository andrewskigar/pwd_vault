package domain

import (
	"maps"
	"slices"
	"strings"
)

// Vault is the decrypted set of entries, keyed case-insensitively by name.
type Vault struct{ entries map[string]Entry }

// NewVault returns an empty vault.
func NewVault() Vault { return Vault{entries: map[string]Entry{}} }

// Add stores e, or returns ErrDuplicate when an entry with the same name key
// exists.
func (v *Vault) Add(e Entry) error {
	key := e.Name().Key()
	if _, ok := v.entries[key]; ok {
		return ErrDuplicate
	}
	if v.entries == nil {
		v.entries = map[string]Entry{}
	}
	v.entries[key] = e
	return nil
}

// Get returns the entry named n, ignoring case, or ErrNotFound.
func (v Vault) Get(n Name) (Entry, error) {
	e, ok := v.entries[n.Key()]
	if !ok {
		return Entry{}, ErrNotFound
	}
	return e, nil
}

// Replace swaps in e for the stored entry with the same name key, or returns
// ErrNotFound. The stored display name is replaced too, so callers keep it by
// building e from the stored entry's fields.
func (v *Vault) Replace(e Entry) error {
	key := e.Name().Key()
	if _, ok := v.entries[key]; !ok {
		return ErrNotFound
	}
	v.entries[key] = e
	return nil
}

// Entries returns a new slice of all entries sorted by name key.
func (v Vault) Entries() []Entry {
	return slices.SortedFunc(maps.Values(v.entries), func(a, b Entry) int {
		return strings.Compare(a.Name().Key(), b.Name().Key())
	})
}

// Len is the number of entries in the vault.
func (v Vault) Len() int { return len(v.entries) }

// Remove deletes the entry named n, ignoring case, or returns ErrNotFound.
func (v *Vault) Remove(n Name) error {
	key := n.Key()
	if _, ok := v.entries[key]; !ok {
		return ErrNotFound
	}
	delete(v.entries, key)
	return nil
}
