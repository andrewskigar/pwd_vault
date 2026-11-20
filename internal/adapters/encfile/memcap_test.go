package encfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"save_pwd/internal/app"
)

func TestMemoryCapLeavesRoomForTheDefault(t *testing.T) {
	if maxMemoryKiB <= DefaultParams().MemoryKiB {
		t.Fatalf("maxMemoryKiB = %d, want more than the default %d KiB", maxMemoryKiB, DefaultParams().MemoryKiB)
	}
}

func TestHeaderMemoryAboveCapIsRejectedWithoutRunningKDF(t *testing.T) {
	tests := []struct {
		name      string
		memoryKiB uint32
	}{
		{name: "257 MiB", memoryKiB: 263168},
		{name: "1 GiB", memoryKiB: 1048576},
	}
	store, spy, original := spiedVault(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forged := bytes.Clone(original)
			binary.BigEndian.PutUint32(forged[offMemory:], tt.memoryKiB)
			spy.calls = nil

			err := loadForged(t, store, forged)

			if !errors.Is(err, app.ErrUnlock) {
				t.Fatalf("Load error = %v, want app.ErrUnlock", err)
			}
			if len(spy.calls) != 0 {
				t.Fatalf("KDF ran with %+v, want the header rejected first", spy.calls)
			}
		})
	}
}

func TestHeaderMemoryUpToCapReachesKDF(t *testing.T) {
	tests := []struct {
		name      string
		memoryKiB uint32
	}{
		{name: "64 MiB", memoryKiB: 65536},
		{name: "256 MiB", memoryKiB: 262144},
	}
	store, spy, original := spiedVault(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forged := bytes.Clone(original)
			binary.BigEndian.PutUint32(forged[offMemory:], tt.memoryKiB)
			spy.calls = nil

			// The spy's fixed key cannot open the forged vault, so Load
			// fails; what matters is that decoding let the header through.
			_ = loadForged(t, store, forged)

			if len(spy.calls) != 1 || spy.calls[0].MemoryKiB != tt.memoryKiB {
				t.Fatalf("KDF calls = %+v, want exactly one with MemoryKiB %d", spy.calls, tt.memoryKiB)
			}
		})
	}
}
