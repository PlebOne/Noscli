package zap

import (
"context"
"encoding/json"
"fmt"
"io"
"net/http"
"net/url"
"strings"
"time"

"github.com/nbd-wtf/go-nostr"
"github.com/nbd-wtf/go-nostr/nip19"
)

// LNURLResponse is the response from a LNURL-pay endpoint
type LNURLResponse struct {
Status       string `json:"status,omitempty"`
Callback     string `json:"callback"`
MinSendable  int64  `json:"minSendable"`
MaxSendable  int64  `json:"maxSendable"`
Metadata     string `json:"metadata"`
Tag          string `json:"tag"`
AllowsNostr  bool   `json:"allowsNostr"`
NostrPubkey  string `json:"nostrPubkey"`
CommentAllowed int  `json:"commentAllowed,omitempty"`
}

// LNURLCallbackResponse is the response from the callback
type LNURLCallbackResponse struct {
PR     string `json:"pr"`     // Payment request (invoice)
Routes []struct{} `json:"routes"`
}

// CreateZapRequest creates a NIP-57 zap request event
func CreateZapRequest(recipientPubkey, eventID, content string, amountSats int64, relays []string, privkey string) (*nostr.Event, error) {
amountMsats := amountSats * 1000

tags := nostr.Tags{
nostr.Tag{"p", recipientPubkey},
nostr.Tag{"amount", fmt.Sprintf("%d", amountMsats)},
}

// Add event ID if this is a zap on a note
if eventID != "" {
tags = append(tags, nostr.Tag{"e", eventID})
}

// Add relays
for _, relay := range relays {
tags = append(tags, nostr.Tag{"relays", relay})
}

pubkey, err := nostr.GetPublicKey(privkey)
if err != nil {
return nil, fmt.Errorf("failed to get public key: %w", err)
}

evt := &nostr.Event{
PubKey:    pubkey,
CreatedAt: nostr.Now(),
Kind:      9734, // Zap request kind
Tags:      tags,
Content:   content,
}

if err := evt.Sign(privkey); err != nil {
return nil, fmt.Errorf("failed to sign zap request: %w", err)
}

return evt, nil
}

// GetLNURL fetches the lightning address metadata
func GetLNURL(lightningAddress string) (string, error) {
// Parse lightning address (user@domain.com)
parts := strings.Split(lightningAddress, "@")
if len(parts) != 2 {
return "", fmt.Errorf("invalid lightning address format")
}

user := parts[0]
domain := parts[1]

// Construct LNURL endpoint
lnurlEndpoint := fmt.Sprintf("https://%s/.well-known/lnurlp/%s", domain, user)

return lnurlEndpoint, nil
}

// FetchLNURLPayInfo fetches LNURL-pay information
func FetchLNURLPayInfo(lnurlEndpoint string) (*LNURLResponse, error) {
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

req, err := http.NewRequestWithContext(ctx, "GET", lnurlEndpoint, nil)
if err != nil {
return nil, fmt.Errorf("failed to create request: %w", err)
}

resp, err := http.DefaultClient.Do(req)
if err != nil {
return nil, fmt.Errorf("failed to fetch LNURL info: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode != 200 {
return nil, fmt.Errorf("LNURL endpoint returned status %d", resp.StatusCode)
}

body, err := io.ReadAll(resp.Body)
if err != nil {
return nil, fmt.Errorf("failed to read response: %w", err)
}

var lnurlResp LNURLResponse
if err := json.Unmarshal(body, &lnurlResp); err != nil {
return nil, fmt.Errorf("failed to parse LNURL response: %w", err)
}

if !lnurlResp.AllowsNostr {
return nil, fmt.Errorf("recipient doesn't support Nostr zaps")
}

return &lnurlResp, nil
}

// GetInvoice gets a lightning invoice for the zap
func GetInvoice(callbackURL string, amountSats int64, zapRequest *nostr.Event) (string, error) {
amountMsats := amountSats * 1000

// Encode zap request as JSON
zapRequestJSON, err := json.Marshal(zapRequest)
if err != nil {
return "", fmt.Errorf("failed to marshal zap request: %w", err)
}

// Build callback URL with parameters
u, err := url.Parse(callbackURL)
if err != nil {
return "", fmt.Errorf("invalid callback URL: %w", err)
}

q := u.Query()
q.Set("amount", fmt.Sprintf("%d", amountMsats))
q.Set("nostr", string(zapRequestJSON))
u.RawQuery = q.Encode()

// Make request
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
if err != nil {
return "", fmt.Errorf("failed to create callback request: %w", err)
}

resp, err := http.DefaultClient.Do(req)
if err != nil {
return "", fmt.Errorf("failed to fetch invoice: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode != 200 {
return "", fmt.Errorf("callback returned status %d", resp.StatusCode)
}

body, err := io.ReadAll(resp.Body)
if err != nil {
return "", fmt.Errorf("failed to read callback response: %w", err)
}

var callbackResp LNURLCallbackResponse
if err := json.Unmarshal(body, &callbackResp); err != nil {
return "", fmt.Errorf("failed to parse callback response: %w", err)
}

if callbackResp.PR == "" {
return "", fmt.Errorf("no payment request in callback response")
}

return callbackResp.PR, nil
}

// GetLightningAddress extracts lightning address from profile metadata
func GetLightningAddress(metadata string) string {
var meta map[string]interface{}
if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
return ""
}

if lud16, ok := meta["lud16"].(string); ok && lud16 != "" {
return lud16
}

if lud06, ok := meta["lud06"].(string); ok && lud06 != "" {
// lud06 is LNURL, need to decode it
if strings.HasPrefix(lud06, "lnurl") {
_, decoded, err := nip19.Decode(lud06)
if err == nil {
if lnurl, ok := decoded.(string); ok {
return lnurl
}
}
}
}

return ""
}
