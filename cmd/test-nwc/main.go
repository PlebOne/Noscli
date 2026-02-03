package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"noscli/pkg/nwc"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: test-nwc <nwc-connection-string>")
		fmt.Println("Example: test-nwc 'nostr+walletconnect://PUBKEY?relay=...'")
		os.Exit(1)
	}

	nwcString := os.Args[1]
	
	log.Println("🔍 Parsing NWC connection string...")
	walletPubkey, relay, _, err := nwc.ParseNWCString(nwcString)
	if err != nil {
		log.Fatalf("❌ Failed to parse NWC string: %v", err)
	}

	log.Printf("✅ Wallet Pubkey: %s", walletPubkey)
	log.Printf("✅ Relay: %s", relay)
	log.Println()

	ctx := context.Background()
	pool := nostr.NewSimplePool(ctx)

	// Test 1: Can we connect to the relay at all?
	log.Println("📡 Test 1: Testing basic relay connection...")
	filters := []nostr.Filter{{
		Kinds:   []int{23195},
		Authors: []string{walletPubkey},
		Limit:   10, // Get last 10 events
	}}

	log.Printf("   Filter: kinds=[23195], authors=[%s...], limit=10", walletPubkey[:8])
	
	eventChan := pool.SubManyEose(ctx, []string{relay}, filters)
	
	timeout := time.After(10 * time.Second)
	eventCount := 0
	
	log.Println("   Waiting for events (10s timeout)...")
	
waitLoop:
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				log.Println("   Channel closed (EOSE received)")
				break waitLoop
			}
			if event.Event == nil {
				continue
			}
			eventCount++
			log.Printf("   📨 Event %d: kind=%d, created=%s, tags=%d", 
				eventCount, event.Event.Kind, 
				time.Unix(int64(event.Event.CreatedAt), 0).Format("15:04:05"),
				len(event.Event.Tags))
		case <-timeout:
			log.Println("   ⏱️  Timeout reached")
			break waitLoop
		}
	}

	if eventCount == 0 {
		log.Println("   ❌ No events received from wallet relay")
		log.Println("   This could mean:")
		log.Println("      - The wallet has never sent any responses")
		log.Println("      - The relay is not working")
		log.Println("      - The relay requires authentication")
		log.Println()
		
		// Test 2: Try a broader subscription
		log.Println("📡 Test 2: Trying broader subscription (any kind)...")
		filters2 := []nostr.Filter{{
			Authors: []string{walletPubkey},
			Limit:   5,
		}}
		
		eventChan2 := pool.SubManyEose(ctx, []string{relay}, filters2)
		timeout2 := time.After(5 * time.Second)
		eventCount2 := 0
		
	waitLoop2:
		for {
			select {
			case event, ok := <-eventChan2:
				if !ok {
					break waitLoop2
				}
				if event.Event == nil {
					continue
				}
				eventCount2++
				log.Printf("   📨 Event %d: kind=%d", eventCount2, event.Event.Kind)
			case <-timeout2:
				break waitLoop2
			}
		}
		
		if eventCount2 == 0 {
			log.Println("   ❌ Still no events with broader filter")
			log.Println("   The relay may be private or the wallet pubkey may be wrong")
		} else {
			log.Printf("   ✅ Found %d events with broader filter", eventCount2)
			log.Println("   Issue: Wallet has events but no kind 23195 responses")
		}
	} else {
		log.Printf("   ✅ Successfully received %d events from wallet relay!", eventCount)
		log.Println("   The relay connection works fine.")
		log.Println("   Issue might be with:")
		log.Println("      - Timing (wallet not responding fast enough)")
		log.Println("      - Request format")
		log.Println("      - Wallet service not processing requests")
	}

	log.Println()
	log.Println("🔚 Test complete")
}
