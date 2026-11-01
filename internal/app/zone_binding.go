package app

import (
	"encoding/hex"
	"fmt"
	"github.com/githubflyideas/unagikuma/internal/config"
	"math"
)

func (s *ZoneStore) validateBinding(binding ZoneBinding) error {
	if binding.SourceNode == "" || binding.SourceNode == s.local {
		if binding.Fingerprint != "" || binding.SourceZoneID != "" || binding.SourceGeneration != 0 {
			return fmt.Errorf("local binding cannot contain upstream fingerprint")
		}
		return nil
	}
	if (binding.SourceZoneID == "") != (binding.SourceGeneration == 0) || (binding.SourceZoneID != "" && !config.ValidZoneID(binding.SourceZoneID)) {
		return fmt.Errorf("remote source zone ID and generation must be specified together")
	}
	fp, err := hex.DecodeString(binding.Fingerprint)
	if !config.ValidName(binding.SourceNode) || err != nil || len(fp) != 32 {
		return fmt.Errorf("remote source requires identity and SHA-256 fingerprint")
	}
	return nil
}

// Rebind is an explicit local administrative operation. The caller MUST perform
// authentication, zone-management authorization and fingerprint confirmation;
// possession of an imported file does not authorize this operation. It changes
// configuration synchronization metadata only, never server execution mode.
func (s *ZoneStore) Rebind(id, expected string, binding ZoneBinding) (ZoneRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	z, err := s.get(id)
	if err != nil {
		return ZoneRecord{}, err
	}
	if expected == "" || expected != z.Revision() {
		return ZoneRecord{}, ErrConflict
	}
	if err := s.validateBinding(binding); err != nil {
		return ZoneRecord{}, err
	}
	if z.Header.Generation == math.MaxUint64 {
		return ZoneRecord{}, fmt.Errorf("generation exhausted")
	}
	z.Binding = binding
	z.Synced = nil
	z.Header.SourceNode = binding.SourceNode
	z.Header.Generation++
	if err = s.persist(z); err != nil {
		return ZoneRecord{}, err
	}
	return z, nil
}
