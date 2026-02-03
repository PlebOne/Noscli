package signer

import (
	"encoding/json"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/nbd-wtf/go-nostr"
)

const (
	Service   = "com.plebsigner.Signer"
	ObjectPath = "/com/plebsigner/Signer"
	Interface = "com.plebsigner.Signer1"
)

type PlebSigner struct {
	conn *dbus.Conn
	obj  dbus.BusObject
}

func NewPlebSigner() (*PlebSigner, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}

	obj := conn.Object(Service, dbus.ObjectPath(ObjectPath))
	return &PlebSigner{conn: conn, obj: obj}, nil
}

func (s *PlebSigner) IsReady() (bool, error) {
	var ready bool
	err := s.obj.Call(Interface+".IsReady", 0).Store(&ready)
	return ready, err
}

type SignerResponse struct {
	Success bool            `json:"success"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"` // Double-encoded JSON
	Error   *string         `json:"error"`
}

type PublicKeyResult struct {
	Type string `json:"type"`
	Npub string `json:"npub"`
	Hex  string `json:"hex"`
}

func (s *PlebSigner) GetPublicKey() (string, error) {
	var jsonStr string
	err := s.obj.Call(Interface+".GetPublicKey", 0).Store(&jsonStr)
	if err != nil {
		return "", err
	}

	var resp SignerResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !resp.Success {
		errMsg := "Unknown error"
		if resp.Error != nil {
			errMsg = *resp.Error
		}
		return "", fmt.Errorf("signer error: %s", errMsg)
	}

	// Result is double-encoded - first decode to get the JSON string
	var resultJSON string
	if err := json.Unmarshal(resp.Result, &resultJSON); err != nil {
		return "", fmt.Errorf("failed to decode result wrapper: %w", err)
	}

	// Now parse the actual public key result
	var pkResult PublicKeyResult
	if err := json.Unmarshal([]byte(resultJSON), &pkResult); err != nil {
		return "", fmt.Errorf("failed to parse public key result: %w", err)
	}

	// Return the hex format
	return pkResult.Hex, nil
}

func (s *PlebSigner) SignEvent(evt *nostr.Event) error {
	// Serialize event to JSON (without ID and Sig)
	eventJSON, err := json.Marshal(evt)
	if err != nil {
		return err
	}

	var respJSON string
	// args: event_json, app_id
	err = s.obj.Call(Interface+".SignEvent", 0, string(eventJSON), "noscli").Store(&respJSON)
	if err != nil {
		return fmt.Errorf("dbus call failed: %w", err)
	}

	var resp SignerResponse
	if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w (response: %s)", err, respJSON)
	}

	if !resp.Success {
		errMsg := "Unknown error"
		if resp.Error != nil {
			errMsg = *resp.Error
		}
		return fmt.Errorf("signer error: %s", errMsg)
	}

	// Result is double-encoded - first decode to get the result object
	var resultStr string
	if err := json.Unmarshal(resp.Result, &resultStr); err != nil {
		return fmt.Errorf("failed to decode result: %w (result: %s)", err, string(resp.Result))
	}

	// Parse the result object which contains type and event_json
	var signResult struct {
		Type      string `json:"type"`
		EventJSON string `json:"event_json"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal([]byte(resultStr), &signResult); err != nil {
		return fmt.Errorf("failed to parse sign result: %w (json: %s)", err, resultStr)
	}

	// Parse the actual signed event from event_json field
	var signedEvt nostr.Event
	if err := json.Unmarshal([]byte(signResult.EventJSON), &signedEvt); err != nil {
		return fmt.Errorf("failed to parse signed event: %w (json: %s)", err, signResult.EventJSON)
	}

	// Replace entire event with signed version from Pleb Signer
	*evt = signedEvt

	return nil
}

func (s *PlebSigner) Nip04Encrypt(recipientPubKey string, plaintext string) (string, error) {
	var respJSON string
	// args: plaintext, recipient_pubkey, app_id
	err := s.obj.Call(Interface+".Nip04Encrypt", 0, plaintext, recipientPubKey, "noscli").Store(&respJSON)
	if err != nil {
		return "", fmt.Errorf("dbus call failed: %w", err)
	}

	var resp SignerResponse
	if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !resp.Success {
		errMsg := "Unknown error"
		if resp.Error != nil {
			errMsg = *resp.Error
		}
		return "", fmt.Errorf("signer error: %s", errMsg)
	}

	// Result is double-encoded - decode the encrypted string
	var encrypted string
	if err := json.Unmarshal(resp.Result, &encrypted); err != nil {
		return "", fmt.Errorf("failed to decode result: %w", err)
	}

	return encrypted, nil
}

func (s *PlebSigner) Nip04Decrypt(senderPubKey string, ciphertext string) (string, error) {
	var respJSON string
	// args: ciphertext, sender_pubkey, app_id
	err := s.obj.Call(Interface+".Nip04Decrypt", 0, ciphertext, senderPubKey, "noscli").Store(&respJSON)
	if err != nil {
		return "", fmt.Errorf("dbus call failed: %w", err)
	}

	var resp SignerResponse
	if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !resp.Success {
		errMsg := "Unknown error"
		if resp.Error != nil {
			errMsg = *resp.Error
		}
		return "", fmt.Errorf("signer error: %s", errMsg)
	}

	// Result is double-encoded - decode the decrypted string
	var plaintext string
	if err := json.Unmarshal(resp.Result, &plaintext); err != nil {
		return "", fmt.Errorf("failed to decode result: %w", err)
	}

	return plaintext, nil
}

func (s *PlebSigner) Close() error {
	return s.conn.Close()
}
