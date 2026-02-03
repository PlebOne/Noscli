# Noscli Development Notes

## Completed Features

### Core Functionality
- [x] Pleb Signer integration via DBus
- [x] Contact list (kind 3) fetching
- [x] Timeline display (kind 1 events)
- [x] Profile metadata (kind 0) caching
- [x] NIP-19 mention decoding (npub, nprofile)

### UI/UX
- [x] Scrollable viewport with cursor sync
- [x] Username display
- [x] Media type indicators
- [x] Vim-style navigation
- [x] Page navigation (pgup/pgdown)

### Media Handling
- [x] Smart URL extraction
- [x] Image viewer selection (imv, feh, sxiv, etc.)
- [x] Video player (mpv)
- [x] GIF animation (chafa, timg, mpv)
- [x] Streaming site detection (YouTube, Twitch, etc.)

## Future Enhancements

### Nice to Have
- [ ] Reply composition
- [ ] Like/Reaction support
- [ ] Repost/Quote support
- [ ] Thread view
- [ ] Profile view
- [ ] Direct messages (NIP-04/NIP-44)
- [ ] Relay management UI
- [ ] Search functionality
- [ ] Notification feed
- [ ] Multiple account switching
- [ ] Custom relay configuration
- [ ] Image preview in terminal (using kitty/sixel protocols)
- [ ] Configurable keybindings
- [ ] Color themes

### Technical Improvements
- [ ] NostrDB integration when Go bindings available
- [ ] Local event caching
- [ ] Optimistic UI updates
- [ ] Better error handling
- [ ] Retry logic for failed fetches
- [ ] Connection status indicator
- [ ] Event validation
- [ ] Rate limiting

## Known Limitations
- Global feed shown if no following list exists
- No local event database (memory only)
- Limited to 50 events per fetch
- No pagination for older events
- Profile fetches are fire-and-forget
