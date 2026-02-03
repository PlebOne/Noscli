# Noscli

A command-line Nostr client built with Go and Charm tools (Bubble Tea, Lip Gloss).

## Features
- **Landing Screen**: Beautiful welcome screen with app info and quick access to client or settings.
- **Dual Authentication**: Choose between Pleb Signer (DBus) or direct nsec key input.
- **Settings**: Configure authentication method, Nostr relays, and Nostr Wallet Connect - add, remove, and manage connections.
- **Pleb Signer Integration**: Secure login using [Pleb Signer](https://github.com/PlebOne/Pleb_Signer) via DBus (optional).
- **Lightning Zaps ⚡**: Send satoshis to support posts you like using Nostr Wallet Connect (NIP-47/NIP-57).
- **Multiple Views**: Tab through Following, DMs, and Notifications with the Tab key.
- **Read/Unread Tracking**: Visual indicators and counts for unread DMs and notifications.
- **Post & Reply**: Compose new posts and reply to existing posts/DMs with full threading support.
- **Thread View**: View full conversation threads with all replies in chronological order.
- **Repost & Quote**: Boost posts or add your thoughts with quote reposts.
- **Encrypted DMs**: Send and receive encrypted direct messages with full support for both NIP-04 (legacy) and NIP-17 (modern gift-wrapped) encryption standards.
- **Following List**: Automatically loads your contact list (kind 3) and shows posts from people you follow.
- **Notifications**: See mentions, replies, and reactions to your posts.
- **Username Display**: Fetches and caches user profiles (kind 0) to show display names instead of pubkeys.
- **Timestamps**: Shows relative time (e.g., "2 hours ago") for recent posts and absolute dates for older ones.
- **Nostr Mentions**: Automatically decodes and displays usernames for `nostr:npub...` and `nostr:nprofile...` links in posts.
- **Event References**: Formats `nostr:nevent...` and `nostr:note...` links as readable event indicators.
- **Scrollable Timeline**: Smooth scrolling through your feed with vim-like keys and page navigation.
- **Multi-URL Support**: Posts with multiple URLs show numbered indicators - press 1-9 to open specific links.
- **Responsive UI**: Footer menu automatically wraps based on terminal width.
- **Smart Media Handling**: 
  - **Animated GIFs**: Opens in `chafa` (terminal), `timg` (terminal), or `mpv` (looped playback)
  - **Images**: Downloads and opens in your preferred image viewer (`imv`, `feh`, `sxiv`, etc.)
  - **Videos**: Opens in `mpv` (if available) for terminal playback, otherwise default video player
  - **Video Streams**: Detects YouTube, Twitch, Twitter/X, Vimeo, and other streaming sites - opens in `mpv`
  - **Links**: Opens in default browser
  - Visual indicators in posts: 📷 (image), 🎞️ (GIF), 🎬 (video), 📺 (stream)
- **Smart URL Extraction**: Properly handles URLs with trailing punctuation.
- **Real-time Refresh**: Press 'r' to fetch new posts/DMs/notifications and merge with existing.
- **Sorted Feed**: Posts sorted by timestamp, newest first.

## Requirements
- Linux
- [Pleb Signer](https://github.com/PlebOne/Pleb_Signer) installed, running, and unlocked.
- At least one key generated in Pleb Signer.
- Go 1.25+
- **Recommended** (for best media experience):
  - `mpv` - For video playback and animated GIFs
  - `imv`, `feh`, or `sxiv` - For image viewing
  - `chafa` or `timg` - For animated GIFs in terminal
  - `yt-dlp` or `youtube-dl` - For streaming video support (mpv will use these automatically)

## Installation

```bash
cd Noscli
go mod tidy
go build -o noscli .
```

## Usage

### First Launch

1. Run `noscli`:
   ```bash
   ./noscli
   ```
2. You'll see the landing screen with two options:
   - **Open Client**: Connect to Nostr network (requires authentication method to be set first)
   - **Settings**: Configure authentication and relays
3. **First time users MUST go to Settings to choose an authentication method**

**Setup Flow:**
```
1. Launch noscli → Landing Screen
2. Select "Settings" (press down arrow, then Enter)
3. Choose authentication method (Pleb Signer or nsec)
4. Press Tab to switch to Relays tab
5. Add/remove relays as needed (or use defaults)
6. Press Enter to start client!
```

### Landing Screen

- Use `↑`/`↓` or `j`/`k` to navigate between options
- Press `Enter` to select
- Press `q` to quit
- If you try to open the client without setting authentication, you'll be redirected to Settings

### Settings Screen

The settings screen has **three tabs** - use `Tab` key to cycle through them:

```
Authentication → Relays → Wallet
```

Press **Tab** to switch tabs! The active tab is highlighted.

#### Authentication Tab (Default - You start here)
Choose how you want to authenticate:
- **Pleb Signer (DBus)**: Uses Pleb Signer for secure key management (recommended for desktop)
  - Requires Pleb Signer to be running and unlocked
  - Keys never leave Pleb Signer
  - Press `Enter` to select
- **Private Key (nsec)**: Enter your nsec1... key directly
  - Press `Enter` and type your nsec key
  - Key is masked while typing for privacy
  - Press `Enter` again to save
  - **Warning**: Your private key will be stored in memory while the app runs
  
**Quick tip**: After setting authentication, press `Tab` to switch to Relays tab!

#### Relays Tab (Press Tab to access)
Configure your Nostr relays:
- `↑`/`↓` or `j`/`k`: Navigate through relay list
- `a`: Add new relay (type URL like wss://relay.example.com and press Enter)
- `d` or `x`: Delete selected relay (must keep at least one)
- `Enter`: Start client with current settings (only available after auth method is set)
- `Tab`: Switch to next tab
- `Esc` or `q`: Back to landing screen

Default relays:
- wss://relay.damus.io
- wss://nos.lol
- wss://relay.nostr.band

#### Wallet Tab (Press Tab twice to access)
Configure Nostr Wallet Connect for zapping:
- Shows current NWC connection status (connected or not)
- `e`: Edit/add NWC connection string
  - Paste your `nostr+walletconnect://...` URI
  - Get from Alby, Mutiny, or other NWC wallets
  - Press `Enter` to save
- `d`: Delete current NWC connection
- `Tab`: Switch to Authentication tab
- `Esc` or `q`: Back to landing screen

**Note**: Zapping requires both NWC setup and nsec authentication.

### Client Navigation

   - `Tab`: Switch between views (Following → DMs → Notifications)
   - `Up` / `k`: Scroll up one line
   - `Down` / `j`: Scroll down one line
   - `Space` / `f` / `PgDown`: Scroll down one page
   - `b` / `PgUp`: Scroll up one page
   - `g`: Jump to top
   - `G`: Jump to bottom
   - `t`: View thread (shows post with all replies)
   - `Enter`: Open first link/media in selected post
   - `1-9`: Open specific numbered link/media (when post has multiple URLs)
   - `r`: Refresh current view (fetches new posts/DMs/notifications and merges with existing)
   - `c`: Compose new post
   - `R`: Reply to selected post/DM
   - `z`: Zap selected post (requires NWC setup)
   - `x`: Repost selected post (boost)
   - `X`: Quote selected post (add your thoughts)
   - `q`: Quit

## Thread View

Press `t` on any post to view the full conversation thread:
- See the original post at the top
- All replies displayed below in chronological order
- Navigate with arrow keys or vim keys (`j`/`k`)
- Press `R` to reply to the thread
- Press `Esc` or `q` to return to timeline

**Note**: Thread view is not available for DMs (privacy protection)

## Posting and Replying

### Compose New Post
- Press `c` to start composing a new post
- Type your message in the text area
- Press `Ctrl+S` to publish
- Press `Esc` to cancel

### Reply to Posts/DMs
- Navigate to any post, notification, or DM
- Press `R` (Shift+r) to reply
- **See reply context**: The original post and author are shown above the compose area
- Type your reply
- Press `Ctrl+S` to send
- Press `Esc` to cancel

**Note**: 
- Replies to posts include proper threading tags ('e' and 'p' tags)
- DM replies are encrypted using NIP-04 via Pleb Signer
- Published posts appear after the next refresh
- Reply context shows up to 200 characters of the original message

## Reposting and Quoting

Boost posts to your followers or add your thoughts with a quote:

- Press `x` on any post to **repost** it (kind 6 boost)
  - Simple boost that shares the post with your followers
  - No additional text needed
  - Published immediately

- Press `X` on any post to **quote** it (quote repost)
  - Opens compose view to add your thoughts
  - Original post is referenced with `nostr:nevent` link
  - Shows quoted post context while composing
  - Press `Ctrl+S` to publish or `Esc` to cancel

**Note**: 
- You cannot repost or quote DMs (privacy protection)
- Reposts and quotes are published to all configured relays
- Quoted posts include proper tags for clients to render them

## Zapping with Lightning ⚡

Noscli supports zapping posts using Nostr Wallet Connect (NWC) - send satoshis to support content you enjoy!

### Setup

1. **Get an NWC connection string** from a wallet that supports it:
   - [Alby](https://getalby.com) - Browser extension and web wallet
   - [Mutiny](https://www.mutinywallet.com) - Self-custodial web wallet
   - Other NWC-compatible wallets

2. **Add NWC to noscli**:
   - Go to Settings (from landing screen)
   - Press `Tab` twice to reach the **Wallet** tab
   - Press `e` to edit
   - Paste your NWC connection string (starts with `nostr+walletconnect://`)
   - Press `Enter` to save

### How to Zap

1. Navigate to any post in your timeline or notifications
2. Press `z` to initiate a zap
3. Enter the amount in satoshis (default: 21 sats)
4. Press `Enter` to confirm and send the zap

The app will:
- Fetch the recipient's lightning address from their profile
- Create a proper NIP-57 zap request
- Get a lightning invoice
- Pay it via your NWC wallet
- Show a success message when complete

**Requirements**:
- NWC connection string configured in Settings → Wallet
- Works with both nsec and Pleb Signer authentication!
- Recipient must have a lightning address in their Nostr profile (lud16 field)

**Note**: The 'z zap' command only appears in the footer when you have NWC connected.

## Multiple URLs in Posts

When a post contains multiple URLs (images, videos, links), they are displayed with numbers:
```
[1] 📷 Image
[2] 🎬 Video  
[3] 🔗 Link
```

- Press `Enter` to open the first URL (quick access)
- Press `1`, `2`, `3`, etc. to open a specific numbered URL
- Up to 9 URLs can be accessed via number keys

## Technical Details

- Uses Pleb Signer's D-Bus API for secure key management
- Three main views accessible via Tab key:
  - **Following**: Posts from people you follow (kind 1 from contact list)
  - **DMs**: Encrypted direct messages (kind 4 NIP-04, kind 1059 NIP-17) - sent and received
  - **Notifications**: Mentions, replies (kind 1 with 'p' tag), and reactions (kind 7)
- Lightning payments via Nostr Wallet Connect:
  - **NIP-47**: Nostr Wallet Connect protocol for secure payment requests
  - **NIP-57**: Zap protocol for creating lightning invoices tied to Nostr events
  - Fetches recipient lightning address from profile metadata
  - Creates zap request events with proper tags
  - Communicates with NWC-compatible wallets (Alby, Mutiny, etc.)
- Robust error handling with panic recovery for relay operations
- Fetches your contact list (kind 3 event) to show posts from people you follow
- Fetches and caches user metadata (kind 0 events) for display names
- Supports NIP-19 encoded identifiers:
  - `npub` - Public keys shown as @username
  - `nprofile` - Profile pointers with relay hints shown as @username
  - `note` - Event IDs shown as 🔗[Event:id...]
  - `nevent` - Event pointers with relay hints shown as 🔗[Event:id...]
- Connects to default relays: `wss://relay.damus.io` and `wss://nos.lol`
- Uses `go-nostr` library for Nostr protocol implementation
- Built with Charm's Bubble Tea TUI framework and Bubbles viewport for smooth scrolling
- Automatically sorts posts by timestamp (newest first)
- Smart media detection:
  - Improved URL extraction with punctuation handling
  - File extension matching for images/videos
  - Domain pattern matching for streaming sites
  - Fallback chain for unavailable viewers
- Profile caching prevents redundant metadata fetches
- Background profile fetching for mentioned users
- Lazy loading: DMs and Notifications are fetched when you first tab to them
- Event deduplication: Refreshing merges new events with existing ones (no duplicates)
- Status messages show count of new items when refreshing
- Read/unread tracking: Blue dot indicators and count badges for new DMs and notifications
- Multi-URL support: Posts with multiple URLs show numbered indicators for easy selection
- Publishing capabilities:
  - Create new posts (kind 1)
  - Reply to posts with proper threading tags
  - Repost (kind 6) and quote repost posts
  - Send encrypted DMs using NIP-04 (with Pleb Signer) or NIP-17 (with nsec)
  - All signing and encryption handled by either Pleb Signer (DBus) or direct nsec key
- DM Encryption - Full support for both standards:
  - **NIP-04** (kind 4): Legacy encrypted DMs - fully supported for send/receive on both auth modes
  - **NIP-17** (kind 1059/14): Modern gift-wrapped DMs with NIP-44 encryption - fully supported with nsec auth
  - **nsec authentication**: Sends NIP-17 (modern), receives both NIP-04 and NIP-17
  - **Pleb Signer authentication**: Sends NIP-04 (legacy), receives NIP-04 (NIP-17 receive pending signer NIP-44 support)
  - Gift-wrapped DMs provide enhanced privacy with temporary keys and randomized timestamps
  - Automatically detects DM type and uses appropriate decryption method
  - Status messages show which encryption standard was used

## Media Support Details

### Images (JPG, PNG, WebP, BMP)
Images are automatically downloaded to a temporary file and opened in your preferred viewer:
1. `imv` - Fast and lightweight image viewer
2. `feh` - Popular minimalist viewer
3. `sxiv` - Simple X Image Viewer
4. `gwenview` - KDE image viewer
5. `eog` - GNOME Eye of GNOME
6. `viewnior` - GTK image viewer
7. `xdg-open` - System default viewer

**Note**: Images are downloaded to `/tmp/noscli-*` and opened locally for compatibility with most viewers.

### Animated GIFs
GIFs are downloaded and opened in the best available viewer:
1. `chafa` - Terminal with animation support
2. `timg` - Terminal image viewer with GIF support  
3. `mpv` - Video player with loop mode for smooth playback
4. Any of the static image viewers above
5. Browser - Fallback option

### Video Files (MP4, WebM, MOV, etc.)
Videos are streamed directly to `mpv` without downloading.

### Video Streams
Streaming URLs are detected by domain patterns and opened in `mpv`:
- YouTube (youtube.com, youtu.be)
- Twitch (twitch.tv)
- Twitter/X videos
- Vimeo
- Streamable
- Dailymotion
- Generic /video/ paths

**Note**: For streaming support, install `yt-dlp` or `youtube-dl`. MPV will automatically use these to handle streaming URLs.

## Note on NostrDB
This client currently uses an in-memory pool for simplicity as there are no official Go bindings for `nostrdb` yet. Future versions may integrate `nostrdb` via CGO or when bindings become available.

## Troubleshooting

**Error: "No keys configured"**
- Open Pleb Signer from your system tray and generate or import a key.

**Error: "The name is not activatable"** or **"Service not found"**
- Make sure Pleb Signer is running: `pgrep pleb-signer`
- Check D-Bus registration: `dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus.ListNames | grep plebsigner`

**Signer is locked**
- Open Pleb Signer and unlock it with your password.

**Video streams don't work in mpv**
- Install `yt-dlp`: `sudo pacman -S yt-dlp` (Arch) or `pip install yt-dlp`
- MPV will automatically use it for streaming URLs

**GIFs don't animate**
- Install `chafa` for terminal playback: `sudo pacman -S chafa` (Arch)
- Or let mpv handle them (already installed)

**DMs show as encrypted text or fail to decrypt**
- **NIP-04** DMs (kind 4) should decrypt automatically with both auth modes
- **NIP-17** DMs (kind 1059) decrypt with nsec authentication
- If using Pleb Signer and see "NIP-17 pending signer support", switch to nsec auth or ask sender to use NIP-04
- Ensure Pleb Signer is running and unlocked (if using that mode)
- NIP-17 provides better privacy with gift wrapping - recommended when using nsec auth
