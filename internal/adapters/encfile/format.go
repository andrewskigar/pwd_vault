package encfile

import (
	"bytes"
	"encoding/binary"

	"golang.org/x/crypto/chacha20poly1305"
)

// File format v1, big-endian. The whole header is the AEAD associated data.
const (
	magic = "SAVE_PWD"

	formatVersion  uint16 = 1
	unlockPassword byte   = 1
	kdfArgon2id    byte   = 1
	aeadXChaCha    byte   = 1

	saltSize   = 16
	nonceSize  = chacha20poly1305.NonceSizeX
	headerSize = 62
	keySize    = chacha20poly1305.KeySize

	offVersion = 8
	offUnlock  = 10
	offKDF     = 11
	offTime    = 12
	offMemory  = 16
	offThreads = 20
	offSalt    = 21
	offAEAD    = 37
	offNonce   = 38

	minFileSize = headerSize + chacha20poly1305.Overhead
	maxFileSize = 64 << 20
)

// Bounds on header params accepted for decryption, so a forged header cannot
// make Load burn unbounded CPU or memory.
const (
	minTime, maxTime           = 1, 16
	minMemoryKiB, maxMemoryKiB = 8, 1 << 20
	minThreads, maxThreads     = 1, 16
)

type header struct {
	params Params
	salt   [saltSize]byte
	nonce  [nonceSize]byte
}

func (h header) encode() []byte {
	b := make([]byte, headerSize)
	copy(b, magic)
	binary.BigEndian.PutUint16(b[offVersion:], formatVersion)
	b[offUnlock] = unlockPassword
	b[offKDF] = kdfArgon2id
	binary.BigEndian.PutUint32(b[offTime:], h.params.Time)
	binary.BigEndian.PutUint32(b[offMemory:], h.params.MemoryKiB)
	b[offThreads] = h.params.Threads
	copy(b[offSalt:], h.salt[:])
	b[offAEAD] = aeadXChaCha
	copy(b[offNonce:], h.nonce[:])
	return b
}

// decodeHeader reports false for anything this version cannot open; callers
// turn that into the generic unlock error.
func decodeHeader(b []byte) (header, bool) {
	if len(b) < headerSize || !bytes.Equal(b[:len(magic)], []byte(magic)) {
		return header{}, false
	}
	if binary.BigEndian.Uint16(b[offVersion:]) != formatVersion ||
		b[offUnlock] != unlockPassword || b[offKDF] != kdfArgon2id || b[offAEAD] != aeadXChaCha {
		return header{}, false
	}
	h := header{params: Params{
		Time:      binary.BigEndian.Uint32(b[offTime:]),
		MemoryKiB: binary.BigEndian.Uint32(b[offMemory:]),
		Threads:   b[offThreads],
	}}
	if !h.params.withinBounds() {
		return header{}, false
	}
	copy(h.salt[:], b[offSalt:offSalt+saltSize])
	copy(h.nonce[:], b[offNonce:offNonce+nonceSize])
	return h, true
}

func (p Params) withinBounds() bool {
	return p.Time >= minTime && p.Time <= maxTime &&
		p.MemoryKiB >= minMemoryKiB && p.MemoryKiB <= maxMemoryKiB &&
		p.Threads >= minThreads && p.Threads <= maxThreads
}
