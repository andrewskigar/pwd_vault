package domain

import "fmt"

// Kind is the sort of resource an entry protects.
type Kind string

const (
	KindSite  Kind = "site"
	KindCard  Kind = "card"
	KindNote  Kind = "note"
	KindOther Kind = "other"
)

// ParseKind accepts exactly one of the lower-case kind names.
func ParseKind(s string) (Kind, error) {
	k := Kind(s)
	if !k.valid() {
		return "", fmt.Errorf("kind must be site, card, note or other: %w", ErrInvalid)
	}
	return k, nil
}

func (k Kind) valid() bool {
	switch k {
	case KindSite, KindCard, KindNote, KindOther:
		return true
	}
	return false
}
