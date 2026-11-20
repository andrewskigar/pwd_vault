// Package app holds the vault use cases and the ports they drive.
package app

import "errors"

var (
	ErrVaultExists   = errors.New("vault already exists")
	ErrVaultNotFound = errors.New("vault not found")
	// ErrUnlock deliberately covers both a wrong password and a damaged file,
	// so the error is no oracle for an attacker.
	ErrUnlock     = errors.New("cannot unlock vault: wrong master password or corrupted file")
	ErrWeakMaster = errors.New("master password must be at least 8 characters")
	ErrNoChanges  = errors.New("no changes requested")
)
