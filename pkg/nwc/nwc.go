package nwc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// NWCClient handles Nostr Wallet Connect (NIP-47) operations
type NWCClient struct {
	walletPubkey string
	relay        string
	secret       string
	pool         *nostr.SimplePool
}

// PayInvoiceRequest is the request to pay a lightning invoice
type PayInvoiceRequest struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// PayInvoiceResponse is the response from paying an invoice
type PayInvoiceResponse struct {
	ResultType string `json:"result_type"`
	Error      *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Result *struct {
		Preimage string `json:"preimage"`
	} `json:"result,omitempty"`
}

// ParseNWCString parses a nostr+walletconnect:// URI
// Format: nostr+walletconnect://[pubkey]?relay=[relay]&secret=[secret]
func ParseNWCString(uri string) (walletPubkey, relay, secret string, err error) {
	if !strings.HasPrefix(uri, "nostr+walletconnect://") {
		return "", "", "", fmt.Errorf("invalid NWC URI: must start with nostr+walletconnect://")
	}

	// Parse URL
	u, err := url.Parse(uri)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to parse URI: %w", err)
	}

	walletPubkey = u.Host
	query := u.Query()
	relay = query.Get("relay")
	secret = query.Get("secret")

	if walletPubkey == "" || relay == "" || secret == "" {
		return "", "", "", fmt.Errorf("missing required parameters (pubkey, relay, or secret)")
	}

	return walletPubkey, relay, secret, nil
}

// NewNWCClient creates a new NWC client from a connection string
func NewNWCClient(connectionString string) (*NWCClient, error) {
	walletPubkey, relay, secret, err := ParseNWCString(connectionString)
	if err != nil {
		return nil, err
	}

	return &NWCClient{
		walletPubkey: walletPubkey,
		relay:        relay,
		secret:       secret,
		pool:         nostr.NewSimplePool(context.Background()),
	}, nil
}

// PayInvoice pays a lightning invoice via NWC
func (c *NWCClient) PayInvoice(ctx context.Context, invoice string) error {
	// Decode the secret (it's an nsec)
	_, sk, err := nip19.Decode(c.secret)
	if err != nil {
		return fmt.Errorf("failed to decode secret: %w", err)
	}

	secretKey := sk.(string)

	// Create the request
	requestContent := PayInvoiceRequest{
		Method: "pay_invoice",
		Params: map[string]interface{}{
			"invoice": invoice,
		},
	}

	contentBytes, err := json.Marshal(requestContent)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Encrypt the content using NIP-04
	sharedSecret, err := nip04.ComputeSharedSecret(c.walletPubkey, secretKey)
	if err != nil {
		return fmt.Errorf("failed to compute shared secret: %w", err)
	}
	
	encryptedContent, err := nip04.Encrypt(string(contentBytes), sharedSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt content: %w", err)
	}

	// Get public key from secret
	pubkey, err := nostr.GetPublicKey(secretKey)
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Create event (kind 23194 for NIP-47)
	evt := nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Now(),
		Kind:      23194,
		Tags: nostr.Tags{
			nostr.Tag{"p", c.walletPubkey},
		},
		Content: encryptedContent,
	}

	// Sign the event
	if err := evt.Sign(secretKey); err != nil {
		return fmt.Errorf("failed to sign event: %w", err)
	}

	// Publish to wallet relay and wait for response
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Subscribe to responses
	since := nostr.Now()
	filters := []nostr.Filter{{
		Kinds:   []int{23195}, // NIP-47 response kind
		Authors: []string{c.walletPubkey},
		Tags:    nostr.TagMap{"p": []string{evt.PubKey}},
		Since:   &since,
	}}

	responseChan := c.pool.SubMany(ctx, []string{c.relay}, filters)

	// Publish the request using PublishMany
	results := c.pool.PublishMany(ctx, []string{c.relay}, evt)
	published := false
	for result := range results {
		if result.Error == nil {
			published = true
		}
	}
	
	if !published {
		return fmt.Errorf("failed to publish request to wallet relay")
	}

	// Wait for response
	select {
	case event := <-responseChan:
		if event.Event == nil {
			return fmt.Errorf("received empty response")
		}

		// Decrypt response
		decrypted, err := nip04.Decrypt(event.Content, sharedSecret)
		if err != nil {
			return fmt.Errorf("failed to decrypt response: %w", err)
		}

		// Parse response
		var response PayInvoiceResponse
		if err := json.Unmarshal([]byte(decrypted), &response); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Check for errors
		if response.Error != nil {
			return fmt.Errorf("wallet error: %s - %s", response.Error.Code, response.Error.Message)
		}

		if response.Result == nil {
			return fmt.Errorf("no result in response")
		}

		return nil

	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for wallet response")
	}
}

// Close closes the NWC client
func (c *NWCClient) Close() {
	// SimplePool doesn't need explicit closing in newer versions
}
