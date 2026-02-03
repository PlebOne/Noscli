#!/bin/bash
# Test NWC connection

echo "Testing NWC wallet connection..."
echo ""
echo "Please paste your NWC connection string:"
read nwc_string

# Extract relay from NWC string
relay=$(echo "$nwc_string" | grep -oP 'relay=\K[^&]+')
pubkey=$(echo "$nwc_string" | sed 's/nostr+walletconnect:\/\///' | cut -d'?' -f1)

echo ""
echo "Wallet Pubkey: $pubkey"
echo "Wallet Relay: $relay"
echo ""
echo "Testing relay connection with websocat..."

if ! command -v websocat &> /dev/null; then
    echo "websocat not found. Install with: cargo install websocat"
    exit 1
fi

echo '["REQ","test",{"kinds":[23195],"authors":["'$pubkey'"],"limit":1}]' | websocat -n1 "$relay"
