package signer

import (
	"errors"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestNilSignerReturnsErrNotConnected(t *testing.T) {
	var s *PlebSigner
	if _, err := s.GetPublicKey(); !errors.Is(err, ErrNotConnected) {
		t.Errorf("GetPublicKey() error = %v, want ErrNotConnected", err)
	}
	if err := s.SignEvent(&nostr.Event{}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("SignEvent() error = %v, want ErrNotConnected", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}
