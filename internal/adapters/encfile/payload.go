package encfile

import (
	"time"

	"save_pwd/internal/domain"
)

// payload is the persistence DTO inside the ciphertext.
type payload struct {
	Entries []entryDTO `json:"entries"`
}

type entryDTO struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	URL       string    `json:"url"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresOn string    `json:"expires_on,omitempty"`
}

func toPayload(v domain.Vault) payload {
	entries := v.Entries()
	p := payload{Entries: make([]entryDTO, 0, len(entries))}
	for _, e := range entries {
		f := e.Fields()
		p.Entries = append(p.Entries, entryDTO{
			Name:      f.Name.String(),
			Kind:      string(f.Kind),
			Username:  f.Username,
			Password:  f.Password.Reveal(),
			URL:       f.URL,
			Notes:     f.Notes,
			CreatedAt: f.CreatedAt.UTC(),
			UpdatedAt: f.UpdatedAt.UTC(),
			ExpiresOn: f.ExpiresOn.String(),
		})
	}
	return p
}

// toVault rebuilds the domain vault through its constructors. It reports
// false for any entry the domain rejects, which Load turns into ErrUnlock.
func (p payload) toVault() (domain.Vault, bool) {
	v := domain.NewVault()
	for _, dto := range p.Entries {
		e, err := dto.toEntry()
		if err != nil {
			return domain.Vault{}, false
		}
		if err := v.Add(e); err != nil {
			return domain.Vault{}, false
		}
	}
	return v, true
}

func (dto entryDTO) toEntry() (domain.Entry, error) {
	name, err := domain.NewName(dto.Name)
	if err != nil {
		return domain.Entry{}, err
	}
	kind, err := domain.ParseKind(dto.Kind)
	if err != nil {
		return domain.Entry{}, err
	}
	pw, err := domain.NewPassword(dto.Password)
	if err != nil {
		return domain.Entry{}, err
	}
	expires, err := domain.ParseDate(dto.ExpiresOn)
	if err != nil {
		return domain.Entry{}, err
	}
	return domain.NewEntry(domain.EntryFields{
		Name:      name,
		Kind:      kind,
		Username:  dto.Username,
		Password:  pw,
		URL:       dto.URL,
		Notes:     dto.Notes,
		CreatedAt: dto.CreatedAt.UTC(),
		UpdatedAt: dto.UpdatedAt.UTC(),
		ExpiresOn: expires,
	})
}
