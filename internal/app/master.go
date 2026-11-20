package app

const masterMask = "********"

// MasterPassword wraps the bytes read from a prompt. It never formats its
// value, and Wipe zeroes the buffer once the command is done with it.
type MasterPassword struct{ b []byte }

// NewMasterPassword takes ownership of b without copying, so Wipe also clears
// the caller's buffer.
func NewMasterPassword(b []byte) MasterPassword { return MasterPassword{b: b} }

// Bytes returns the secret for key derivation.
func (m MasterPassword) Bytes() []byte { return m.b }

// Wipe zeroes the secret in place.
func (m MasterPassword) Wipe() { clear(m.b) }

func (m MasterPassword) String() string { return masterMask }

func (m MasterPassword) GoString() string { return "app.MasterPassword(" + masterMask + ")" }
