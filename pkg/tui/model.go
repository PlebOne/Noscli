package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip44"
	"github.com/nbd-wtf/go-nostr/nip59"
	"noscli/pkg/signer"
)

const Version = "1.0.0"

type sessionState int

const (
	stateLanding sessionState = iota
	stateSettings
	stateConnecting
	stateLoadingFollows
	stateTimeline
	stateComposing
	stateThread
	stateError
)

type composeMode int

const (
	composePost composeMode = iota
	composeReply
	composeDM
	composeQuote
)

type viewMode int

const (
	viewFollowing viewMode = iota
	viewDMs
	viewNotifications
)

type Model struct {
	state         sessionState
	currentView   viewMode
	signer        *signer.PlebSigner
	pubKey        string
	privKey       string             // Private key if using nsec auth
	npub          string
	events        []nostr.Event      // Timeline events
	dms           []nostr.Event      // DM events
	notifications []nostr.Event      // Notification events
	following     []string           // List of pubkeys we follow
	cursor        int
	viewport      viewport.Model
	ready         bool
	err           error
	width         int
	height        int
	statusMsg     string
	pool          *nostr.SimplePool
	relays        []string
	eventLines    []int              // Track which line each event starts at
	userCache     map[string]string  // pubkey -> display name
	lastEventTime nostr.Timestamp    // Latest event timestamp for refresh
	lastDMTime    nostr.Timestamp    // Latest DM timestamp for refresh
	lastNotifTime nostr.Timestamp    // Latest notification timestamp for refresh
	// Compose mode
	textarea      textarea.Model
	composing     composeMode
	replyingTo    *nostr.Event       // Event we're replying to
	// Thread view
	threadRoot    *nostr.Event       // Root event of thread being viewed
	threadEvents  []nostr.Event      // All events in thread
	// Landing/Settings
	landingChoice int                // 0 = Open Client, 1 = Settings
	settingsMenu  int                // 0 = Auth Method, 1 = Relays
	settingsCursor int               // Which relay is selected in settings
	editingRelay  bool               // Whether we're editing a relay
	newRelayInput string             // Input for new relay
	// Auth settings
	authMethod    string             // "pleb_signer" or "nsec" or ""
	nsecKey       string             // User's nsec key if authMethod is "nsec"
	editingNsec   bool               // Whether we're editing nsec input
}

func NewModel() Model {
	s, err := signer.NewPlebSigner()
	
	// Initialize textarea for composing
	ta := textarea.New()
	ta.Placeholder = "What's on your mind?"
	ta.Focus()
	ta.CharLimit = 5000
	ta.SetWidth(60)
	ta.SetHeight(5)
	
	m := Model{
		state:       stateLanding,
		currentView: viewFollowing,
		signer:      s,
		pool:        nostr.NewSimplePool(context.Background()),
		relays:      []string{"wss://relay.damus.io", "wss://nos.lol", "wss://relay.nostr.band"}, // Default relays
		cursor:      0,
		userCache:   make(map[string]string),
		textarea:    ta,
		landingChoice: 0,
	}
	if err != nil {
		m.state = stateError
		m.err = err
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.state == stateError {
		return nil
	}
	// Start at landing screen, don't connect yet
	return nil
}

func (m *Model) updateContent() {
	var content strings.Builder
	
	// Thread view rendering
	if m.state == stateThread && m.threadRoot != nil {
		// Render root post
		content.WriteString(lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214")).
			Render("━━━ Thread Root ━━━"))
		content.WriteString("\n\n")
		content.WriteString(m.renderEvent(*m.threadRoot, false))
		content.WriteString("\n\n")
		
		if len(m.threadEvents) > 0 {
			content.WriteString(lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("214")).
				Render(fmt.Sprintf("━━━ Replies (%d) ━━━", len(m.threadEvents))))
			content.WriteString("\n\n")
			
			for _, evt := range m.threadEvents {
				content.WriteString(m.renderEvent(evt, false))
				content.WriteString("\n")
			}
		} else {
			content.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Render("No replies yet"))
			content.WriteString("\n")
		}
		
		m.viewport.SetContent(content.String())
		return
	}
	
	// Get the current event list based on view
	var currentEvents []nostr.Event
	switch m.currentView {
	case viewFollowing:
		currentEvents = m.events
	case viewDMs:
		currentEvents = m.dms
	case viewNotifications:
		currentEvents = m.notifications
	}
	
	m.eventLines = make([]int, len(currentEvents))
	currentLine := 0
	
	if len(currentEvents) == 0 {
		content.WriteString("No posts yet. Press 'r' to refresh.\n")
	} else {
		for i, evt := range currentEvents {
			m.eventLines[i] = currentLine
			rendered := m.renderEvent(evt, i == m.cursor)
			content.WriteString(rendered)
			content.WriteString("\n")
			// Count lines in rendered event
			currentLine += strings.Count(rendered, "\n") + 2 // +2 for the newline after
		}
	}
	
	m.viewport.SetContent(content.String())
}

func (m *Model) getCurrentEvents() []nostr.Event {
	switch m.currentView {
	case viewFollowing:
		return m.events
	case viewDMs:
		return m.dms
	case viewNotifications:
		return m.notifications
	default:
		return m.events
	}
}

func (m *Model) scrollToCursor() {
	if m.cursor >= len(m.eventLines) || len(m.eventLines) == 0 {
		return
	}
	
	targetLine := m.eventLines[m.cursor]
	
	// Calculate viewport boundaries
	viewportTop := m.viewport.YOffset
	viewportBottom := viewportTop + m.viewport.Height
	
	// If cursor is above viewport, scroll up
	if targetLine < viewportTop {
		m.viewport.SetYOffset(targetLine)
	}
	
	// If cursor is below viewport, scroll down
	// Account for event height (approximate 4 lines per event)
	eventHeight := 4
	if targetLine + eventHeight > viewportBottom {
		m.viewport.SetYOffset(targetLine - m.viewport.Height + eventHeight)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		headerHeight := 5  // Header + tabs + spacing
		// Footer height depends on width - estimate based on commands
		footerHeight := m.estimateFooterHeight()
		verticalMarginHeight := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = headerHeight
			m.ready = true
			m.updateContent()
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}
		m.updateContent()

	case tea.KeyMsg:
		// Handle landing screen
		if m.state == stateLanding {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "up", "k":
				m.landingChoice = 0
			case "down", "j":
				m.landingChoice = 1
			case "enter":
				if m.landingChoice == 0 {
					// Open Client - check if auth method is set
					if m.authMethod == "" {
						// Force user to settings to choose auth
						m.state = stateSettings
						m.statusMsg = "Please choose an authentication method first"
					} else {
						m.state = stateConnecting
						if m.authMethod == "pleb_signer" {
							return m, connectToPlebSignerCmd(m.signer)
						} else {
							return m, connectWithNsecCmd(m.nsecKey)
						}
					}
				} else {
					// Go to Settings
					m.state = stateSettings
				}
			}
			return m, nil
		}
		
		// Handle settings screen
		if m.state == stateSettings {
			if m.editingNsec {
				switch msg.String() {
				case "esc":
					m.editingNsec = false
					m.nsecKey = ""
				case "enter":
					if strings.TrimSpace(m.nsecKey) != "" {
						// Validate nsec format
						trimmedKey := strings.TrimSpace(m.nsecKey)
						if strings.HasPrefix(trimmedKey, "nsec1") {
							m.editingNsec = false
							m.statusMsg = "Decoding nsec key..."
							// Decode and save the nsec key
							return m, connectWithNsecCmd(trimmedKey)
						} else {
							m.statusMsg = "❌ Invalid nsec format - must start with nsec1"
							m.nsecKey = ""
							m.editingNsec = false
						}
					} else {
						m.statusMsg = "❌ Please enter an nsec key"
						m.editingNsec = false
					}
				case "backspace":
					if len(m.nsecKey) > 0 {
						m.nsecKey = m.nsecKey[:len(m.nsecKey)-1]
					}
				default:
					// Handle paste events and single character input
					input := msg.String()
					if len(input) > 0 {
						// Accept both single chars and multi-char paste
						m.nsecKey += input
					}
				}
				return m, nil
			}
			
			if m.editingRelay {
				switch msg.String() {
				case "esc":
					m.editingRelay = false
					m.newRelayInput = ""
				case "enter":
					if strings.TrimSpace(m.newRelayInput) != "" {
						// Add new relay
						m.relays = append(m.relays, strings.TrimSpace(m.newRelayInput))
						m.newRelayInput = ""
						m.editingRelay = false
					}
				case "backspace":
					if len(m.newRelayInput) > 0 {
						m.newRelayInput = m.newRelayInput[:len(m.newRelayInput)-1]
					}
				default:
					// Handle paste events and single character input
					input := msg.String()
					if len(input) > 0 {
						// Accept both single chars and multi-char paste
						m.newRelayInput += input
					}
				}
				return m, nil
			}
			
			// Settings menu navigation
			switch msg.String() {
			case "q", "esc":
				// Back to landing
				m.state = stateLanding
			case "tab":
				// Switch between auth and relays menu
				if m.settingsMenu == 0 {
					m.settingsMenu = 1
					m.settingsCursor = 0
				} else {
					m.settingsMenu = 0
					m.settingsCursor = 0
				}
			case "up", "k":
				if m.settingsCursor > 0 {
					m.settingsCursor--
				}
			case "down", "j":
				if m.settingsMenu == 1 && m.settingsCursor < len(m.relays)-1 {
					m.settingsCursor++
				} else if m.settingsMenu == 0 && m.settingsCursor < 1 {
					m.settingsCursor++
				}
			case "enter":
				if m.settingsMenu == 0 {
					// Auth method selection
					if m.settingsCursor == 0 {
						m.authMethod = "pleb_signer"
					} else {
						m.editingNsec = true
						m.nsecKey = ""
					}
				} else if m.settingsMenu == 1 && m.authMethod != "" {
					// Start client if auth method is set
					m.state = stateConnecting
					if m.authMethod == "pleb_signer" {
						return m, connectToPlebSignerCmd(m.signer)
					} else {
						return m, connectWithNsecCmd(m.nsecKey)
					}
				}
			case "a":
				// Add new relay (only in relays menu)
				if m.settingsMenu == 1 {
					m.editingRelay = true
					m.newRelayInput = ""
				}
			case "d", "x":
				// Delete selected relay (only in relays menu)
				if m.settingsMenu == 1 && m.settingsCursor < len(m.relays) && len(m.relays) > 1 {
					m.relays = append(m.relays[:m.settingsCursor], m.relays[m.settingsCursor+1:]...)
					if m.settingsCursor >= len(m.relays) {
						m.settingsCursor = len(m.relays) - 1
					}
				}
			}
			return m, nil
		}
		
		// Handle thread view mode
		if m.state == stateThread {
			switch msg.String() {
			case "esc", "q":
				// Exit thread view
				m.state = stateTimeline
				m.threadRoot = nil
				m.threadEvents = nil
				m.statusMsg = "Back to timeline"
				m.updateContent()
				return m, nil
			case "R":
				// Reply to thread root
				m.state = stateComposing
				m.composing = composeReply
				m.replyingTo = m.threadRoot
				m.textarea.Placeholder = "Write your reply... (Ctrl+S to send, Esc to cancel)"
				username := m.userCache[m.threadRoot.PubKey]
				if username == "" {
					username = m.threadRoot.PubKey[:8] + "..."
				}
				m.statusMsg = "Replying to @" + username
				m.textarea.Focus()
				return m, nil
			case "up", "k":
				m.viewport.LineUp(1)
			case "down", "j":
				m.viewport.LineDown(1)
			case "pgup", "b":
				m.viewport.ViewUp()
			case "pgdown", "f", " ":
				m.viewport.ViewDown()
			case "g":
				m.viewport.GotoTop()
			case "G":
				m.viewport.GotoBottom()
			}
			return m, nil
		}
		
		// Handle composing mode separately
		if m.state == stateComposing {
			switch msg.String() {
			case "esc":
				// Cancel composing
				m.state = stateTimeline
				m.textarea.Reset()
				m.replyingTo = nil
				m.statusMsg = "Cancelled"
				return m, nil
			case "ctrl+s":
				// Submit post
				content := m.textarea.Value()
				if strings.TrimSpace(content) == "" {
					m.statusMsg = "Cannot send empty message"
					return m, nil
				}
				
				m.state = stateTimeline
				m.textarea.Reset()
				
				switch m.composing {
				case composePost:
					m.statusMsg = "Publishing post..."
					return m, publishPostCmd(m.signer, m.pool, m.relays, m.pubKey, content, nil, m.authMethod, m.privKey)
				case composeReply:
					m.statusMsg = "Publishing reply..."
					return m, publishPostCmd(m.signer, m.pool, m.relays, m.pubKey, content, m.replyingTo, m.authMethod, m.privKey)
				case composeQuote:
					m.statusMsg = "Publishing quote..."
					return m, publishQuoteCmd(m.signer, m.pool, m.relays, m.pubKey, content, m.replyingTo)
				case composeDM:
					if m.replyingTo == nil {
						m.statusMsg = "Error: No DM recipient"
						return m, nil
					}
					m.statusMsg = "Sending DM..."
					return m, publishDMCmd(m.signer, m.pool, m.relays, m.pubKey, m.replyingTo.PubKey, content, m.authMethod, m.privKey)
				}
			default:
				var cmd tea.Cmd
				m.textarea, cmd = m.textarea.Update(msg)
				return m, cmd
			}
		}
		
		// Normal mode key handling
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "c":
			// Start composing new post
			m.state = stateComposing
			m.composing = composePost
			m.replyingTo = nil
			m.textarea.Placeholder = "What's on your mind? (Ctrl+S to send, Esc to cancel)"
			m.textarea.Focus()
			m.statusMsg = "Composing new post"
			return m, nil
		case "R":
			// Reply to selected post or DM
			currentEvents := m.getCurrentEvents()
			if len(currentEvents) > 0 && m.cursor < len(currentEvents) {
				evt := currentEvents[m.cursor]
				m.state = stateComposing
				m.replyingTo = &evt
				
				if m.currentView == viewDMs {
					m.composing = composeDM
					m.textarea.Placeholder = "Reply to DM... (Ctrl+S to send, Esc to cancel)"
					username := m.userCache[evt.PubKey]
					if username == "" {
						username = evt.PubKey[:8] + "..."
					}
					m.statusMsg = "Replying to @" + username
				} else {
					m.composing = composeReply
					m.textarea.Placeholder = "Write your reply... (Ctrl+S to send, Esc to cancel)"
					username := m.userCache[evt.PubKey]
					if username == "" {
						username = evt.PubKey[:8] + "..."
					}
					m.statusMsg = "Replying to @" + username
				}
				
				m.textarea.Focus()
				return m, nil
			}
		case "tab":
			// Switch to next view
			m.cursor = 0 // Reset cursor when switching views
			switch m.currentView {
			case viewFollowing:
				m.currentView = viewDMs
				if len(m.dms) == 0 {
					m.statusMsg = "Loading DMs..."
					m.updateContent()
					return m, fetchDMsCmd(m.pool, m.relays, m.pubKey)
				}
			case viewDMs:
				m.currentView = viewNotifications
				if len(m.notifications) == 0 {
					m.statusMsg = "Loading notifications..."
					m.updateContent()
					return m, fetchNotificationsCmd(m.pool, m.relays, m.pubKey)
				}
			case viewNotifications:
				m.currentView = viewFollowing
			}
			m.updateContent()
			m.viewport.GotoTop()
		case "r":
			// Refresh (unless in DMs or Notifications where 'r' might be confused)
			if m.currentView == viewFollowing {
				m.statusMsg = "Refreshing..."
				return m, fetchEventsCmd(m.pool, m.relays, m.following)
			}
		case "x":
			// Simple repost (kind 6)
			currentEvents := m.getCurrentEvents()
			if len(currentEvents) > 0 && m.cursor < len(currentEvents) {
				evt := currentEvents[m.cursor]
				if m.currentView != viewDMs { // Can't repost DMs
					m.statusMsg = "Reposting..."
					return m, repostCmd(m.signer, m.pool, m.relays, m.pubKey, &evt)
				}
			}
		case "X":
			// Quote repost (kind 1 with quote)
			currentEvents := m.getCurrentEvents()
			if len(currentEvents) > 0 && m.cursor < len(currentEvents) {
				evt := currentEvents[m.cursor]
				if m.currentView != viewDMs { // Can't quote DMs
					m.state = stateComposing
					m.composing = composeQuote
					m.replyingTo = &evt
					m.textarea.Placeholder = "Add your thoughts... (Ctrl+S to send, Esc to cancel)"
					username := m.userCache[evt.PubKey]
					if username == "" {
						username = evt.PubKey[:8] + "..."
					}
					m.statusMsg = "Quote reposting @" + username
					m.textarea.Focus()
					return m, nil
				}
			}
		case "g":
			// Go to top
			m.cursor = 0
			m.updateContent()
			m.viewport.GotoTop()
		case "G":
			// Go to bottom
			currentEvents := m.getCurrentEvents()
			if len(currentEvents) > 0 {
				m.cursor = len(currentEvents) - 1
			}
			m.updateContent()
			m.viewport.GotoBottom()
		case "t":
			// View thread
			currentEvents := m.getCurrentEvents()
			if len(currentEvents) > 0 && m.cursor < len(currentEvents) {
				evt := currentEvents[m.cursor]
				if m.currentView != viewDMs { // Don't show threads for DMs
					m.state = stateThread
					m.threadRoot = &evt
					m.statusMsg = "Loading thread..."
					return m, fetchThreadCmd(m.pool, m.relays, &evt)
				}
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.updateContent()
				m.scrollToCursor()
			}
		case "down", "j":
			currentEvents := m.getCurrentEvents()
			if m.cursor < len(currentEvents)-1 {
				m.cursor++
				m.updateContent()
				m.scrollToCursor()
			}
		case "pgup", "b":
			// Move cursor up by viewport height worth of events
			step := m.viewport.Height / 4 // Approximate 4 lines per event
			if step < 1 {
				step = 1
			}
			m.cursor -= step
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.updateContent()
			m.scrollToCursor()
		case "pgdown", "f", " ":
			// Move cursor down by viewport height worth of events
			step := m.viewport.Height / 4 // Approximate 4 lines per event
			if step < 1 {
				step = 1
			}
			currentEvents := m.getCurrentEvents()
			m.cursor += step
			if m.cursor >= len(currentEvents) {
				m.cursor = len(currentEvents) - 1
			}
			m.updateContent()
			m.scrollToCursor()
		case "enter":
			currentEvents := m.getCurrentEvents()
			if len(currentEvents) > 0 && m.cursor < len(currentEvents) {
				evt := currentEvents[m.cursor]
				url := extractFirstURL(evt.Content)
				if url != "" {
					go OpenMedia(url)
					m.statusMsg = "Opening: " + url
				}
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Open specific URL by number
			currentEvents := m.getCurrentEvents()
			if len(currentEvents) > 0 && m.cursor < len(currentEvents) {
				evt := currentEvents[m.cursor]
				urls := extractAllURLs(evt.Content)
				index := int(msg.String()[0] - '1') // Convert '1'-'9' to 0-8
				if index < len(urls) {
					go OpenMedia(urls[index])
					m.statusMsg = fmt.Sprintf("Opening [%d]: %s", index+1, urls[index])
				}
			}
		}
		// Don't update content again here since we already did it above
		// Basic scrolling could be added here if we had a viewport

	case nsecAuthMsg:
		m.authMethod = "nsec"
		m.pubKey = msg.pubKey
		m.privKey = msg.privKey
		m.nsecKey = "" // Clear the input field
		if m.pubKey == "" {
			m.state = stateError
			m.err = fmt.Errorf("received empty pubkey")
			return m, nil
		}
		npub, err := nip19.EncodePublicKey(m.pubKey)
		if err != nil {
			m.state = stateError
			m.err = err
			return m, nil
		}
		m.npub = npub
		m.statusMsg = "Authenticated with nsec! Loading..."
		m.state = stateLoadingFollows
		return m, fetchFollowingCmd(m.pool, m.relays, m.pubKey)
	
	case pubKeyMsg:
		m.pubKey = msg.pubKey
		if m.pubKey == "" {
			m.state = stateError
			m.err = fmt.Errorf("received empty pubkey")
			return m, nil
		}
		npub, err := nip19.EncodePublicKey(m.pubKey)
		if err != nil {
			m.state = stateError
			m.err = err
			return m, nil
		}
		m.npub = npub
		m.state = stateLoadingFollows
		m.statusMsg = "Loading following list..."
		return m, fetchFollowingCmd(m.pool, m.relays, m.pubKey)
	
	case signerReadyMsg:
		m.pubKey = msg.pubkey
		if m.pubKey == "" {
			m.state = stateError
			m.err = fmt.Errorf("received empty pubkey")
			return m, nil
		}
		npub, err := nip19.EncodePublicKey(m.pubKey)
		if err != nil {
			m.state = stateError
			m.err = err
			return m, nil
		}
		m.npub = npub
		m.state = stateLoadingFollows
		m.statusMsg = "Loading following list..."
		return m, fetchFollowingCmd(m.pool, m.relays, m.pubKey)

	case followingMsg:
		m.following = msg.pubkeys
		m.state = stateTimeline
		if len(m.following) == 0 {
			m.statusMsg = "No following list found. Showing global feed."
		} else {
			m.statusMsg = fmt.Sprintf("Connected. Following %d people.", len(m.following))
		}
		return m, tea.Batch(
			fetchEventsCmd(m.pool, m.relays, m.following),
			fetchProfilesCmd(m.pool, m.relays, m.following),
		)

	case errMsg:
		m.state = stateError
		m.err = msg.err

	case eventsMsg:
		// Merge new events with existing ones
		newCount := 0
		eventMap := make(map[string]nostr.Event)
		
		// Add existing events to map
		for _, evt := range m.events {
			eventMap[evt.ID] = evt
		}
		
		// Add new events to map (and count them)
		for _, evt := range msg.events {
			if _, exists := eventMap[evt.ID]; !exists {
				newCount++
			}
			eventMap[evt.ID] = evt
			
			// Track latest timestamp
			if evt.CreatedAt > m.lastEventTime {
				m.lastEventTime = evt.CreatedAt
			}
		}
		
		// Convert map back to slice
		m.events = make([]nostr.Event, 0, len(eventMap))
		for _, evt := range eventMap {
			m.events = append(m.events, evt)
		}
		
		// Sort by timestamp, newest first
		sort.Slice(m.events, func(i, j int) bool {
			return m.events[i].CreatedAt.Time().After(m.events[j].CreatedAt.Time())
		})
		
		if newCount > 0 {
			m.statusMsg = fmt.Sprintf("Loaded %d new posts (%d total)", newCount, len(m.events))
		} else {
			m.statusMsg = fmt.Sprintf("No new posts (%d total)", len(m.events))
		}
		m.updateContent()
	
	case dmsMsg:
		// Merge new DMs with existing ones
		newCount := 0
		dmMap := make(map[string]nostr.Event)
		
		// Add existing DMs to map
		for _, evt := range m.dms {
			dmMap[evt.ID] = evt
		}
		
		// Add new DMs to map (and count them)
		for _, evt := range msg.events {
			if _, exists := dmMap[evt.ID]; !exists {
				newCount++
			}
			dmMap[evt.ID] = evt
			
			// Track latest timestamp
			if evt.CreatedAt > m.lastDMTime {
				m.lastDMTime = evt.CreatedAt
			}
		}
		
		// Convert map back to slice
		m.dms = make([]nostr.Event, 0, len(dmMap))
		for _, evt := range dmMap {
			m.dms = append(m.dms, evt)
		}
		
		sort.Slice(m.dms, func(i, j int) bool {
			return m.dms[i].CreatedAt.Time().After(m.dms[j].CreatedAt.Time())
		})
		
		if newCount > 0 {
			m.statusMsg = fmt.Sprintf("Loaded %d new DMs (%d total)", newCount, len(m.dms))
		} else {
			m.statusMsg = fmt.Sprintf("No new DMs (%d total)", len(m.dms))
		}
		m.updateContent()
		
		// Extract unique pubkeys from DMs and fetch their profiles
		pubkeys := make(map[string]bool)
		for _, evt := range m.dms {
			pubkeys[evt.PubKey] = true
			// Also get recipients from 'p' tags
			for _, tag := range evt.Tags {
				if len(tag) >= 2 && tag[0] == "p" {
					pubkeys[tag[1]] = true
				}
			}
		}
		
		var pubkeyList []string
		for pk := range pubkeys {
			if _, cached := m.userCache[pk]; !cached {
				pubkeyList = append(pubkeyList, pk)
			}
		}
		
		if len(pubkeyList) > 0 {
			return m, fetchProfilesCmd(m.pool, m.relays, pubkeyList)
		}
	
	case notificationsMsg:
		// Merge new notifications with existing ones
		newCount := 0
		notifMap := make(map[string]nostr.Event)
		
		// Add existing notifications to map
		for _, evt := range m.notifications {
			notifMap[evt.ID] = evt
		}
		
		// Add new notifications to map (and count them)
		for _, evt := range msg.events {
			if _, exists := notifMap[evt.ID]; !exists {
				newCount++
			}
			notifMap[evt.ID] = evt
			
			// Track latest timestamp
			if evt.CreatedAt > m.lastNotifTime {
				m.lastNotifTime = evt.CreatedAt
			}
		}
		
		// Convert map back to slice
		m.notifications = make([]nostr.Event, 0, len(notifMap))
		for _, evt := range notifMap {
			m.notifications = append(m.notifications, evt)
		}
		
		sort.Slice(m.notifications, func(i, j int) bool {
			return m.notifications[i].CreatedAt.Time().After(m.notifications[j].CreatedAt.Time())
		})
		
		if newCount > 0 {
			m.statusMsg = fmt.Sprintf("Loaded %d new notifications (%d total)", newCount, len(m.notifications))
		} else {
			m.statusMsg = fmt.Sprintf("No new notifications (%d total)", len(m.notifications))
		}
		m.updateContent()
		
		// Extract unique pubkeys from notifications and fetch their profiles
		pubkeys := make(map[string]bool)
		for _, evt := range m.notifications {
			pubkeys[evt.PubKey] = true
		}
		
		var pubkeyList []string
		for pk := range pubkeys {
			if _, cached := m.userCache[pk]; !cached {
				pubkeyList = append(pubkeyList, pk)
			}
		}
		
		if len(pubkeyList) > 0 {
			return m, fetchProfilesCmd(m.pool, m.relays, pubkeyList)
		}
	
	case profilesMsg:
		for pubkey, name := range msg.profiles {
			m.userCache[pubkey] = name
		}
		m.updateContent() // Refresh to show new names
		
	case threadEventsMsg:
		m.threadEvents = msg.events
		m.statusMsg = fmt.Sprintf("Thread loaded (%d replies)", len(msg.events))
		m.updateContent()
		
	case publishSuccessMsg:
		// Use custom status if provided, otherwise determine from context
		if msg.status != "" {
			m.statusMsg = msg.status
		} else {
			// Determine what was published based on current state
			action := "Published"
			if msg.eventID != "" {
				// Check the last status to determine action type
				if strings.Contains(m.statusMsg, "Reposting") {
					action = "Reposted"
				} else if strings.Contains(m.statusMsg, "quote") {
					action = "Quote published"
				} else if strings.Contains(m.statusMsg, "reply") {
					action = "Reply sent"
				} else if strings.Contains(m.statusMsg, "DM") {
					action = "DM sent"
				}
			}
			m.statusMsg = action + " successfully! ✓"
		}
		m.replyingTo = nil
		// Refresh current view to show new post
		switch m.currentView {
		case viewFollowing:
			return m, fetchEventsCmd(m.pool, m.relays, m.following)
		case viewDMs:
			return m, fetchDMsCmd(m.pool, m.relays, m.pubKey)
		case viewNotifications:
			return m, fetchNotificationsCmd(m.pool, m.relays, m.pubKey)
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.state == stateLanding {
		return m.renderLanding()
	}
	
	if m.state == stateSettings {
		return m.renderSettings()
	}
	
	if m.state == stateError {
		var msg string
		if m.authMethod == "nsec" {
			msg = fmt.Sprintf("Error with nsec authentication: %v\n\n", m.err)
			msg += "Please check your nsec key and try again.\n"
			msg += "\nPress q to quit or Esc to go back to settings."
		} else {
			msg = fmt.Sprintf("Error connecting to Pleb Signer: %v\n\n", m.err)
			if strings.Contains(m.err.Error(), "No keys configured") {
				msg += "Pleb Signer is running but the key is locked or unavailable.\n\n"
				msg += "Please:\n"
				msg += "1. Click the Pleb Signer icon in your system tray\n"
				msg += "2. Unlock it with your password\n"
				msg += "3. Make sure a key is active and available\n"
				msg += "4. Try running noscli again\n"
			} else if strings.Contains(m.err.Error(), "not activatable") || strings.Contains(m.err.Error(), "ServiceUnknown") {
				msg += "Pleb Signer is not running.\n\n"
				msg += "Please start Pleb Signer:\n"
				msg += "  pleb-signer\n"
			} else {
				msg += "Make sure Pleb Signer is running and unlocked.\n"
			}
			msg += "\nPress q to quit."
		}
		return msg
	}

	if m.state == stateConnecting {
		return "Connecting to Pleb Signer..."
	}

	if m.state == stateLoadingFollows {
		return "Loading your following list..."
	}
	
	if m.state == stateThread {
		// Show thread view
		header := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Render(fmt.Sprintf("Noscli - %s", m.statusMsg))
		
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render("R reply • ↑↓/k/j scroll • Esc/q back")
		
		return fmt.Sprintf("%s\n%s\n\n%s", header, m.viewport.View(), footer)
	}
	
	if m.state == stateComposing {
		// Show compose view
		header := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Render(fmt.Sprintf("Noscli - %s", m.statusMsg))
		
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render("Ctrl+S send • Esc cancel")
		
		// Show reply context if replying or quoting
		var contextView string
		if m.replyingTo != nil {
			username := m.userCache[m.replyingTo.PubKey]
			if username == "" {
				username = m.replyingTo.PubKey[:8] + "..."
			}
			
			// Process and truncate content for display
			content := m.processContent(m.replyingTo.Content)
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			
			contextStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).
				Padding(0, 1).
				Width(60)
			
			var contextLabel string
			if m.composing == composeQuote {
				contextLabel = "Quoting @"
			} else {
				contextLabel = "Replying to @"
			}
			
			contextView = contextStyle.Render(fmt.Sprintf("%s%s:\n%s", contextLabel, username, content)) + "\n\n"
		}
		
		return fmt.Sprintf("%s\n\n%s%s\n\n%s", header, contextView, m.textarea.View(), footer)
	}

	if !m.ready {
		return "Initializing..."
	}

	// Tab bar
	tabStyle := lipgloss.NewStyle().Padding(0, 2)
	activeTabStyle := tabStyle.Copy().Bold(true).Foreground(lipgloss.Color("205"))
	inactiveTabStyle := tabStyle.Copy().Foreground(lipgloss.Color("241"))

	followingTab := inactiveTabStyle.Render("Following")
	dmsTab := inactiveTabStyle.Render("DMs")
	notificationsTab := inactiveTabStyle.Render("Notifications")

	switch m.currentView {
	case viewFollowing:
		followingTab = activeTabStyle.Render("Following")
	case viewDMs:
		dmsTab = activeTabStyle.Render("DMs")
	case viewNotifications:
		notificationsTab = activeTabStyle.Render("Notifications")
	}

	tabs := lipgloss.JoinHorizontal(lipgloss.Top, followingTab, dmsTab, notificationsTab)

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render(fmt.Sprintf("Noscli - %s", m.statusMsg))

	// Create responsive footer that wraps based on width
	footer := m.renderFooter()

	return fmt.Sprintf("%s\n%s\n\n%s\n\n%s", header, tabs, m.viewport.View(), footer)
}

func (m *Model) renderFooter() string {
	commands := []string{
		"c compose",
		"R reply",
		"x repost",
		"X quote",
		"t thread",
		"tab switch",
		"↑/k ↓/j nav",
		"space/f/b page",
		"g/G top/bot",
		"r refresh",
		"enter/1-9 open",
		"q quit",
	}
	
	// Calculate available width (leave some margin)
	availableWidth := m.width - 4
	if availableWidth < 40 {
		availableWidth = 40
	}
	
	// Build lines that fit within width
	var lines []string
	currentLine := ""
	
	for i, cmd := range commands {
		separator := " • "
		if i == 0 {
			separator = ""
		}
		
		testLine := currentLine + separator + cmd
		// Rough estimate: each character is ~1 width
		if len(testLine) > availableWidth && currentLine != "" {
			// Start new line
			lines = append(lines, currentLine)
			currentLine = cmd
		} else {
			currentLine = testLine
		}
	}
	
	// Add final line
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	
	// Join lines and apply style
	footerText := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(footerText)
}

func (m *Model) estimateFooterHeight() int {
	// Estimate how many lines the footer will take
	// Each command is roughly 10-15 chars, plus separators
	totalChars := 80 // Approximate total length of all commands with separators
	availableWidth := m.width - 4
	if availableWidth < 40 {
		availableWidth = 40
	}
	
	lines := (totalChars / availableWidth) + 1
	if lines < 1 {
		lines = 1
	}
	if lines > 4 {
		lines = 4 // Cap at 4 lines
	}
	
	return lines + 1 // Add 1 for padding
}

func (m *Model) renderEvent(evt nostr.Event, selected bool) string {
	borderColor := lipgloss.Color("63") // Default purple
	if selected {
		borderColor = lipgloss.Color("205") // Pinkish for selected
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(60)

	// Decrypt DMs if this is a kind 4 (NIP-04) or kind 1059 (NIP-17 gift wrap)
	displayContent := evt.Content
	if evt.Kind == 4 {
		// NIP-04: Old encrypted DMs
		// Determine the other party's pubkey for decryption
		otherPubkey := ""
		if evt.PubKey == m.pubKey {
			// We sent this, decrypt with recipient's key
			for _, tag := range evt.Tags {
				if len(tag) >= 2 && tag[0] == "p" {
					otherPubkey = tag[1]
					break
				}
			}
		} else {
			// We received this, decrypt with sender's key
			otherPubkey = evt.PubKey
		}
		
		if otherPubkey != "" {
			decrypted, err := m.decryptDM(evt.Content, otherPubkey)
			if err == nil {
				displayContent = decrypted
			} else {
				displayContent = fmt.Sprintf("[🔒 NIP-04 Encrypted - Decrypt error: %v]", err)
			}
		}
	} else if evt.Kind == 1059 {
		// NIP-17: Gift-wrapped DM
		decrypted, err := m.unwrapGiftWrapDM(evt)
		if err == nil {
			displayContent = decrypted
		} else {
			displayContent = fmt.Sprintf("[🎁 NIP-17 Gift-wrap - Unwrap error: %v]", err)
		}
	}
	
	content := m.processContent(displayContent)
	
	// Extract and annotate all URLs with numbers
	urls := extractAllURLs(evt.Content)
	if len(urls) > 0 {
		content += "\n"
		for i, url := range urls {
			lowerURL := strings.ToLower(url)
			mediaType := "🔗 Link"
			
			if strings.HasSuffix(lowerURL, ".jpg") || strings.HasSuffix(lowerURL, ".jpeg") || 
			   strings.HasSuffix(lowerURL, ".png") || strings.HasSuffix(lowerURL, ".webp") {
				mediaType = "📷 Image"
			} else if strings.HasSuffix(lowerURL, ".gif") {
				mediaType = "🎞️  GIF"
			} else if strings.HasSuffix(lowerURL, ".mp4") || strings.HasSuffix(lowerURL, ".webm") ||
			          strings.HasSuffix(lowerURL, ".mkv") {
				mediaType = "🎬 Video"
			} else if strings.Contains(lowerURL, "youtube.com") || strings.Contains(lowerURL, "youtu.be") ||
			          strings.Contains(lowerURL, "twitch.tv") || strings.Contains(lowerURL, "vimeo.com") {
				mediaType = "📺 Stream"
			}
			
			if len(urls) > 1 {
				content += fmt.Sprintf("\n[%d] %s", i+1, mediaType)
			} else {
				content += fmt.Sprintf("\n%s", mediaType)
			}
		}
	}

	// Get display name
	displayName := m.userCache[evt.PubKey]
	if displayName == "" {
		displayName = evt.PubKey[:8] + "..."
	}
	
	// Format timestamp
	timestamp := formatTimestamp(evt.CreatedAt.Time())
	
	// Create header with username and timestamp
	headerStyle := lipgloss.NewStyle().Bold(true)
	timestampStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	
	header := fmt.Sprintf("%s  %s", 
		headerStyle.Render("@"+displayName),
		timestampStyle.Render(timestamp))
	
	if selected {
		header = "> " + header
	}
	
	return style.Render(fmt.Sprintf("%s\n%s", header, content))
}

func formatTimestamp(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	
	// Less than a minute
	if diff < time.Minute {
		return "just now"
	}
	
	// Less than an hour
	if diff < time.Hour {
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	}
	
	// Less than a day
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	
	// Less than a week
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
	
	// Less than a year
	if diff < 365*24*time.Hour {
		// Show date
		return t.Format("Jan 2")
	}
	
	// Older than a year
	return t.Format("Jan 2, 2006")
}

func (m *Model) processContent(content string) string {
	// Replace nostr: URIs with readable text
	words := strings.Fields(content)
	var processed []string
	
	for _, word := range words {
		cleanWord := strings.TrimRight(word, ".,;:!?)]")
		trailingPunct := word[len(cleanWord):]
		
		if strings.HasPrefix(cleanWord, "nostr:npub") || strings.HasPrefix(cleanWord, "nostr:nprofile") {
			// Extract the NIP-19 identifier
			nip19Str := strings.TrimPrefix(cleanWord, "nostr:")
			
			if pubkey := m.extractPubkeyFromNip19(nip19Str); pubkey != "" {
				username := m.userCache[pubkey]
				if username == "" {
					username = pubkey[:8] + "..."
				}
				// Replace with @username
				processed = append(processed, "@"+username+trailingPunct)
				// Queue fetching this profile if not cached
				if _, exists := m.userCache[pubkey]; !exists {
					go m.fetchSingleProfile(pubkey)
				}
			} else {
				processed = append(processed, word)
			}
		} else if strings.HasPrefix(cleanWord, "nostr:nevent") || strings.HasPrefix(cleanWord, "nostr:note") {
			// Handle event references
			nip19Str := strings.TrimPrefix(cleanWord, "nostr:")
			eventID := m.extractEventIDFromNip19(nip19Str)
			if eventID != "" {
				// Show as a link indicator
				processed = append(processed, "🔗[Event:"+eventID[:8]+"...]"+trailingPunct)
			} else {
				processed = append(processed, word)
			}
		} else {
			processed = append(processed, word)
		}
	}
	
	return strings.Join(processed, " ")
}

func (m *Model) extractEventIDFromNip19(nip19Str string) string {
	if strings.HasPrefix(nip19Str, "note") {
		_, data, err := nip19.Decode(nip19Str)
		if err == nil {
			if eventID, ok := data.(string); ok {
				return eventID
			}
		}
	} else if strings.HasPrefix(nip19Str, "nevent") {
		_, data, err := nip19.Decode(nip19Str)
		if err == nil {
			if eventPointer, ok := data.(nostr.EventPointer); ok {
				return eventPointer.ID
			}
		}
	}
	return ""
}

func (m *Model) extractPubkeyFromNip19(nip19Str string) string {
	if strings.HasPrefix(nip19Str, "npub") {
		_, data, err := nip19.Decode(nip19Str)
		if err == nil {
			if pubkey, ok := data.(string); ok {
				return pubkey
			}
		}
	} else if strings.HasPrefix(nip19Str, "nprofile") {
		_, data, err := nip19.Decode(nip19Str)
		if err == nil {
			if profile, ok := data.(nostr.ProfilePointer); ok {
				return profile.PublicKey
			}
		}
	}
	return ""
}

func (m *Model) fetchSingleProfile(pubkey string) {
	defer func() {
		if r := recover(); r != nil {
			// Catch panics from relay operations
		}
	}()
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := nostr.Filter{
		Kinds:   []int{0},
		Authors: []string{pubkey},
		Limit:   1,
	}

	eventsChannel := m.pool.SubManyEose(ctx, m.relays, []nostr.Filter{filter})
	for event := range eventsChannel {
		if event.Event == nil {
			continue
		}
		
		var metadata struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
			Username    string `json:"username"`
		}
		
		if err := json.Unmarshal([]byte(event.Event.Content), &metadata); err == nil {
			displayName := metadata.DisplayName
			if displayName == "" {
				displayName = metadata.Name
			}
			if displayName == "" {
				displayName = metadata.Username
			}
			if displayName != "" {
				m.userCache[pubkey] = displayName
			}
		}
		break
	}
}

// Commands and Msg types

type signerReadyMsg struct {
	pubkey string // hex format
}

type followingMsg struct {
	pubkeys []string
}

type profilesMsg struct {
	profiles map[string]string // pubkey -> display name
}

type errMsg struct {
	err error
}

func extractFirstURL(text string) string {
	urls := extractAllURLs(text)
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func extractAllURLs(text string) []string {
	var urls []string
	words := strings.Fields(text)
	for _, word := range words {
		// Remove trailing punctuation
		cleanWord := strings.TrimRight(word, ".,;:!?)]")
		if strings.HasPrefix(cleanWord, "http://") || strings.HasPrefix(cleanWord, "https://") {
			urls = append(urls, cleanWord)
		}
	}
	return urls
}

func checkSignerCmd(s *signer.PlebSigner) tea.Cmd {
	return func() tea.Msg {
		ready, err := s.IsReady()
		if err != nil {
			return errMsg{err}
		}
		if !ready {
			return errMsg{fmt.Errorf("signer not ready (is it unlocked?)")}
		}
		pubkey, err := s.GetPublicKey()
		if err != nil {
			return errMsg{err}
		}
		return signerReadyMsg{pubkey}
	}
}

func fetchFollowingCmd(pool *nostr.SimplePool, relays []string, pubKey string) tea.Cmd {
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				// Catch panics from relay operations
			}
		}()
		
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Fetch kind 3 (contact list)
		filter := nostr.Filter{
			Kinds:   []int{3}, // Kind 3 = Contact List
			Authors: []string{pubKey},
			Limit:   1,
		}

		var following []string
		events := pool.SubManyEose(ctx, relays, []nostr.Filter{filter})
		
		for event := range events {
			if event.Event == nil {
				continue
			}
			// Extract pubkeys from p tags
			for _, tag := range event.Event.Tags {
				if len(tag) >= 2 && tag[0] == "p" {
					following = append(following, tag[1])
				}
			}
			break // Only need the first/latest contact list
		}

		return followingMsg{following}
	}
}

func fetchProfilesCmd(pool *nostr.SimplePool, relays []string, pubkeys []string) tea.Cmd {
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				// Catch panics from relay operations
			}
		}()
		
		if len(pubkeys) == 0 {
			return profilesMsg{profiles: make(map[string]string)}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Fetch kind 0 (metadata) events
		filter := nostr.Filter{
			Kinds:   []int{0}, // Kind 0 = Metadata
			Authors: pubkeys,
		}

		profiles := make(map[string]string)
		eventsChannel := pool.SubManyEose(ctx, relays, []nostr.Filter{filter})
		
		for event := range eventsChannel {
			if event.Event == nil {
				continue
			}
			
			// Parse metadata JSON
			var metadata struct {
				Name        string `json:"name"`
				DisplayName string `json:"display_name"`
				Username    string `json:"username"`
			}
			
			if err := json.Unmarshal([]byte(event.Event.Content), &metadata); err == nil {
				// Prefer display_name, then name, then username
				displayName := metadata.DisplayName
				if displayName == "" {
					displayName = metadata.Name
				}
				if displayName == "" {
					displayName = metadata.Username
				}
				if displayName != "" {
					profiles[event.Event.PubKey] = displayName
				}
			}
		}

		return profilesMsg{profiles}
	}
}

type eventsMsg struct {
	events []nostr.Event
}

type dmsMsg struct {
	events []nostr.Event
}

type notificationsMsg struct {
	events []nostr.Event
}

type threadEventsMsg struct {
	events []nostr.Event
}

func fetchThreadCmd(pool *nostr.SimplePool, relays []string, rootEvent *nostr.Event) tea.Cmd {
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				// Catch panics from relay operations
			}
		}()
		
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Fetch all replies to this event (events with 'e' tag pointing to rootEvent.ID)
		filter := nostr.Filter{
			Kinds: []int{nostr.KindTextNote}, // Kind 1 (notes/posts)
			Tags: nostr.TagMap{
				"e": []string{rootEvent.ID}, // Events that reference this ID
			},
			Limit: 100,
		}

		var events []nostr.Event
		eventsChannel := pool.SubManyEose(ctx, relays, []nostr.Filter{filter})
		
		for event := range eventsChannel {
			if event.Event == nil {
				continue
			}
			events = append(events, *event.Event)
		}
		
		// Sort by timestamp (oldest first for thread reading)
		sort.Slice(events, func(i, j int) bool {
			return events[i].CreatedAt < events[j].CreatedAt
		})
		
		return threadEventsMsg{events: events}
	}
}

func fetchEventsCmd(pool *nostr.SimplePool, relays []string, following []string) tea.Cmd {
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				// Catch panics from relay operations
			}
		}()
		
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		filter := nostr.Filter{
			Kinds: []int{nostr.KindTextNote}, // Kind 1 (notes/posts)
			Limit: 50,
		}

		// If we have a following list, filter by those authors
		if len(following) > 0 {
			filter.Authors = following
		}

		var events []nostr.Event
		eventsChannel := pool.SubManyEose(ctx, relays, []nostr.Filter{filter})
		
		for event := range eventsChannel {
			if event.Event == nil {
				continue
			}
			events = append(events, *event.Event)
		}

		return eventsMsg{events}
	}
}

func fetchDMsCmd(pool *nostr.SimplePool, relays []string, pubKey string) tea.Cmd {
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				// Catch panics from relay operations
			}
		}()
		
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Fetch kind 4 (NIP-04) and kind 1059 (NIP-17 gift wrap) DMs where we are recipient
		filter := nostr.Filter{
			Kinds: []int{4, 1059}, // Kind 4 = NIP-04, Kind 1059 = NIP-17 gift wrap
			Limit: 50,
		}
		
		// We want DMs where we're either the author OR in the 'p' tag (recipient)
		filter.Tags = nostr.TagMap{"p": []string{pubKey}}

		var dms []nostr.Event
		eventsChannel := pool.SubManyEose(ctx, relays, []nostr.Filter{filter})
		
		for event := range eventsChannel {
			if event.Event == nil {
				continue
			}
			dms = append(dms, *event.Event)
		}

		// Also get DMs we sent
		filter2 := nostr.Filter{
			Kinds:   []int{4},
			Authors: []string{pubKey},
			Limit:   50,
		}
		
		eventsChannel2 := pool.SubManyEose(ctx, relays, []nostr.Filter{filter2})
		for event := range eventsChannel2 {
			if event.Event == nil {
				continue
			}
			dms = append(dms, *event.Event)
		}

		return dmsMsg{dms}
	}
}

func fetchNotificationsCmd(pool *nostr.SimplePool, relays []string, pubKey string) tea.Cmd {
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				// Catch panics from relay operations
			}
		}()
		
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Fetch notifications: mentions (kind 1 with 'p' tag), replies, reactions (kind 7)
		var notifications []nostr.Event
		
		// Get mentions and replies (kind 1 events that tag us)
		filter1 := nostr.Filter{
			Kinds: []int{1}, // Kind 1 = Text note
			Tags:  nostr.TagMap{"p": []string{pubKey}},
			Limit: 30,
		}
		
		eventsChannel := pool.SubManyEose(ctx, relays, []nostr.Filter{filter1})
		for event := range eventsChannel {
			if event.Event == nil {
				continue
			}
			notifications = append(notifications, *event.Event)
		}
		
		// Get reactions to our posts (kind 7)
		filter2 := nostr.Filter{
			Kinds: []int{7}, // Kind 7 = Reaction
			Tags:  nostr.TagMap{"p": []string{pubKey}},
			Limit: 20,
		}
		
		eventsChannel2 := pool.SubManyEose(ctx, relays, []nostr.Filter{filter2})
		for event := range eventsChannel2 {
			if event.Event == nil {
				continue
			}
			notifications = append(notifications, *event.Event)
		}

		return notificationsMsg{notifications}
	}
}

func OpenMedia(url string) {
	lowerURL := strings.ToLower(url)
	
	// Check for video streaming sites (without file extensions)
	isStreamingSite := strings.Contains(lowerURL, "youtube.com") ||
		strings.Contains(lowerURL, "youtu.be") ||
		strings.Contains(lowerURL, "twitch.tv") ||
		strings.Contains(lowerURL, "vimeo.com") ||
		strings.Contains(lowerURL, "twitter.com/") ||
		strings.Contains(lowerURL, "x.com/") ||
		strings.Contains(lowerURL, "/video/") ||
		strings.Contains(lowerURL, "streamable.com") ||
		strings.Contains(lowerURL, "dailymotion.com")
	
	// Check file extensions
	isGif := strings.HasSuffix(lowerURL, ".gif")
	
	isImage := strings.HasSuffix(lowerURL, ".jpg") || 
		strings.HasSuffix(lowerURL, ".jpeg") || 
		strings.HasSuffix(lowerURL, ".png") || 
		strings.HasSuffix(lowerURL, ".webp") ||
		strings.HasSuffix(lowerURL, ".bmp")
	
	isVideo := strings.HasSuffix(lowerURL, ".mp4") || 
		strings.HasSuffix(lowerURL, ".webm") || 
		strings.HasSuffix(lowerURL, ".mov") || 
		strings.HasSuffix(lowerURL, ".avi") || 
		strings.HasSuffix(lowerURL, ".mkv") ||
		strings.HasSuffix(lowerURL, ".m4v") ||
		strings.HasSuffix(lowerURL, ".flv")
	
	// Handle GIFs specially - download and view locally
	if isGif {
		// Download to temp file
		tmpFile, err := downloadToTemp(url, ".gif")
		if err != nil {
			// Fallback to browser on download error
			exec.Command("xdg-open", url).Start()
			return
		}
		
		// Try terminal image viewers that support GIFs
		if _, err := exec.LookPath("chafa"); err == nil {
			exec.Command("chafa", "--animate=on", tmpFile).Start()
			return
		}
		if _, err := exec.LookPath("timg"); err == nil {
			exec.Command("timg", tmpFile).Start()
			return
		}
		// mpv can play animated GIFs
		if _, err := exec.LookPath("mpv"); err == nil {
			exec.Command("mpv", "--loop", tmpFile).Start()
			return
		}
		// Try image viewers with downloaded file
		viewers := []string{"imv", "feh", "sxiv", "gwenview", "eog", "viewnior"}
		for _, viewer := range viewers {
			if _, err := exec.LookPath(viewer); err == nil {
				exec.Command(viewer, tmpFile).Start()
				return
			}
		}
		// Fallback to xdg-open with local file
		exec.Command("xdg-open", tmpFile).Start()
		return
	}
	
	// Handle videos and streaming sites
	if isVideo || isStreamingSite {
		// mpv handles URLs directly for videos and streaming
		if _, err := exec.LookPath("mpv"); err == nil {
			exec.Command("mpv", url).Start()
			return
		}
		// Fallback to xdg-open
		exec.Command("xdg-open", url).Start()
		return
	}
	
	// Handle static images - download first
	if isImage {
		// Download to temp file
		ext := ".jpg"
		if strings.HasSuffix(lowerURL, ".png") {
			ext = ".png"
		} else if strings.HasSuffix(lowerURL, ".webp") {
			ext = ".webp"
		} else if strings.HasSuffix(lowerURL, ".bmp") {
			ext = ".bmp"
		}
		
		tmpFile, err := downloadToTemp(url, ext)
		if err != nil {
			// Fallback to browser on download error
			exec.Command("xdg-open", url).Start()
			return
		}
		
		// Try common image viewers with local file
		viewers := []string{"imv", "feh", "sxiv", "gwenview", "eog", "viewnior"}
		for _, viewer := range viewers {
			if _, err := exec.LookPath(viewer); err == nil {
				exec.Command(viewer, tmpFile).Start()
				return
			}
		}
		// Fallback to xdg-open with local file
		exec.Command("xdg-open", tmpFile).Start()
		return
	}
	
	// For other links, just open in default handler
	exec.Command("xdg-open", url).Start()
}

func downloadToTemp(url string, ext string) (string, error) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "noscli-*"+ext)
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()
	
	// Download the file
	resp, err := http.Get(url)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}
	
	// Copy to temp file
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}
	
	return tmpFile.Name(), nil
}

type publishSuccessMsg struct {
	eventID string
	status  string // Optional custom status message
}

func publishPostCmd(signer *signer.PlebSigner, pool *nostr.SimplePool, relays []string, pubKey string, content string, replyTo *nostr.Event, authMethod string, privKey string) tea.Cmd {
	return func() tea.Msg {
		// Create event WITHOUT pubkey - let Pleb Signer set everything
		evt := nostr.Event{
			Kind:      nostr.KindTextNote,
			Content:   content,
			CreatedAt: nostr.Now(),
			Tags:      nostr.Tags{},
		}
		
		// Add reply tags if this is a reply
		if replyTo != nil {
			// Add 'e' tag for event being replied to
			evt.Tags = append(evt.Tags, nostr.Tag{"e", replyTo.ID, "", "reply"})
			// Add 'p' tag for author being replied to
			evt.Tags = append(evt.Tags, nostr.Tag{"p", replyTo.PubKey})
			
			// If original event has 'e' tags, add root tag
			for _, tag := range replyTo.Tags {
				if len(tag) >= 2 && tag[0] == "e" {
					evt.Tags = append(evt.Tags, nostr.Tag{"e", tag[1], "", "root"})
					break
				}
			}
		}
		
		// Sign event 
		var err error
		if authMethod == "nsec" && privKey != "" {
			err = evt.Sign(privKey)
		} else {
			err = signer.SignEvent(&evt)
		}
		if err != nil {
			return errMsg{fmt.Errorf("failed to sign event: %w", err)}
		}
		
		// Publish to relays
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		results := pool.PublishMany(ctx, relays, evt)
		
		// Collect results and track successes/failures
		successCount := 0
		var lastErr error
		for result := range results {
			if result.Error == nil {
				successCount++
			} else {
				lastErr = result.Error
			}
		}
		
		if successCount == 0 {
			if lastErr != nil {
				return errMsg{fmt.Errorf("failed to publish: %w", lastErr)}
			}
			return errMsg{fmt.Errorf("failed to publish to any relay (no results)")}
		}
		
		return publishSuccessMsg{eventID: evt.ID}
	}
}

func publishDMCmd(signer *signer.PlebSigner, pool *nostr.SimplePool, relays []string, pubKey string, recipientPubKey string, content string, authMethod string, privKey string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		// Use NIP-17 (gift wrap) if using nsec, NIP-04 if using Pleb Signer
		if authMethod == "nsec" && privKey != "" {
			// Send NIP-17 gift-wrapped DM with NIP-44 encryption
			// Create the rumor (unsigned event with actual DM content)
			rumor := nostr.Event{
				Kind:      nostr.KindDirectMessage, // Kind 14
				Content:   content,
				CreatedAt: nostr.Now(),
				Tags:      nostr.Tags{nostr.Tag{"p", recipientPubKey}},
				PubKey:    pubKey,
			}
			rumor.ID = rumor.GetID()
			
			// Create encrypt function for gift wrapping
			encrypt := func(plaintext string, targetPubkey string) (string, error) {
				conversationKey, err := nip44.GenerateConversationKey(targetPubkey, privKey)
				if err != nil {
					return "", err
				}
				return nip44.Encrypt(plaintext, conversationKey)
			}
			
			// Create sign function
			sign := func(evt *nostr.Event) error {
				return evt.Sign(privKey)
			}
			
			// Gift wrap to recipient
			giftWrapToRecipient, err := nip59.GiftWrap(
				rumor,
				recipientPubKey,
				func(plaintext string) (string, error) {
					return encrypt(plaintext, recipientPubKey)
				},
				sign,
				nil,
			)
			if err != nil {
				return errMsg{fmt.Errorf("failed to create gift wrap for recipient: %w", err)}
			}
			
			// Gift wrap to ourselves (so we can see it in our DM list)
			giftWrapToUs, err := nip59.GiftWrap(
				rumor,
				pubKey,
				func(plaintext string) (string, error) {
					return encrypt(plaintext, pubKey)
				},
				sign,
				nil,
			)
			if err != nil {
				return errMsg{fmt.Errorf("failed to create gift wrap for self: %w", err)}
			}
			
			// Publish both gift wraps
			successCount := 0
			var lastErr error
			
			// Publish to recipient
			results := pool.PublishMany(ctx, relays, giftWrapToRecipient)
			for result := range results {
				if result.Error == nil {
					successCount++
				} else {
					lastErr = result.Error
				}
			}
			
			// Publish to ourselves
			results = pool.PublishMany(ctx, relays, giftWrapToUs)
			for result := range results {
				if result.Error == nil {
					successCount++
				} else {
					lastErr = result.Error
				}
			}
			
			if successCount == 0 {
				if lastErr != nil {
					return errMsg{fmt.Errorf("failed to publish NIP-17 DM: %w", lastErr)}
				}
				return errMsg{fmt.Errorf("failed to publish NIP-17 DM to any relay")}
			}
			
			return publishSuccessMsg{eventID: giftWrapToRecipient.ID, status: "DM sent (NIP-17) ✓"}
		} else {
			// Send NIP-04 encrypted DM (for Pleb Signer)
			var encrypted string
			var err error
			
			// Use Pleb Signer
			encrypted, err = signer.Nip04Encrypt(recipientPubKey, content)
			if err != nil {
				return errMsg{fmt.Errorf("failed to encrypt DM: %w", err)}
			}
			
			// Create DM event
			evt := nostr.Event{
				Kind:      nostr.KindEncryptedDirectMessage,
				Content:   encrypted,
				CreatedAt: nostr.Now(),
				Tags:      nostr.Tags{nostr.Tag{"p", recipientPubKey}},
			}
			
			// Sign event with Pleb Signer
			err = signer.SignEvent(&evt)
			if err != nil {
				return errMsg{fmt.Errorf("failed to sign DM: %w", err)}
			}
			
			// Publish to relays
			results := pool.PublishMany(ctx, relays, evt)
			
			// Collect results and track successes/failures
			successCount := 0
			var lastErr error
			for result := range results {
				if result.Error == nil {
					successCount++
				} else {
					lastErr = result.Error
				}
			}
			
			if successCount == 0 {
				if lastErr != nil {
					return errMsg{fmt.Errorf("failed to publish NIP-04 DM: %w", lastErr)}
				}
				return errMsg{fmt.Errorf("failed to publish DM to any relay (no results)")}
			}
			
			return publishSuccessMsg{eventID: evt.ID, status: "DM sent (NIP-04) ✓"}
		}
	}
}

func repostCmd(signer *signer.PlebSigner, pool *nostr.SimplePool, relays []string, pubKey string, originalEvent *nostr.Event) tea.Cmd {
return func() tea.Msg {
// Create kind 6 repost event
evt := nostr.Event{
Kind:      nostr.KindRepost,
Content:   "",
CreatedAt: nostr.Now(),
Tags: nostr.Tags{
nostr.Tag{"e", originalEvent.ID},
nostr.Tag{"p", originalEvent.PubKey},
},
}

// Sign event
err := signer.SignEvent(&evt)
if err != nil {
return errMsg{fmt.Errorf("failed to sign repost: %w", err)}
}

// Publish to relays
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

results := pool.PublishMany(ctx, relays, evt)

successCount := 0
var lastErr error
for result := range results {
if result.Error == nil {
successCount++
} else {
lastErr = result.Error
}
}

if successCount == 0 {
if lastErr != nil {
return errMsg{fmt.Errorf("failed to repost: %w", lastErr)}
}
return errMsg{fmt.Errorf("failed to repost to any relay")}
}

return publishSuccessMsg{eventID: evt.ID}
}
}


func publishQuoteCmd(signer *signer.PlebSigner, pool *nostr.SimplePool, relays []string, pubKey string, content string, quotedEvent *nostr.Event) tea.Cmd {
return func() tea.Msg {
// Create kind 1 quote post with nostr: reference
nevent, _ := nip19.EncodeEvent(quotedEvent.ID, []string{}, quotedEvent.PubKey)

// Add the quote reference to content
quoteContent := content + "\nnostr:" + nevent

evt := nostr.Event{
Kind:      nostr.KindTextNote,
Content:   quoteContent,
CreatedAt: nostr.Now(),
Tags: nostr.Tags{
nostr.Tag{"e", quotedEvent.ID, "", "mention"},
nostr.Tag{"p", quotedEvent.PubKey},
},
}

// Sign event
err := signer.SignEvent(&evt)
if err != nil {
return errMsg{fmt.Errorf("failed to sign quote: %w", err)}
}

// Publish to relays
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

results := pool.PublishMany(ctx, relays, evt)

successCount := 0
var lastErr error
for result := range results {
if result.Error == nil {
successCount++
} else {
lastErr = result.Error
}
}

if successCount == 0 {
if lastErr != nil {
return errMsg{fmt.Errorf("failed to publish quote: %w", lastErr)}
}
return errMsg{fmt.Errorf("failed to publish quote to any relay")}
}

return publishSuccessMsg{eventID: evt.ID}
}
}

func (m Model) renderLanding() string {
titleStyle := lipgloss.NewStyle().
Bold(true).
Foreground(lipgloss.Color("205")).
Align(lipgloss.Center).
Width(m.width)

versionStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("241")).
Align(lipgloss.Center).
Width(m.width)

authorStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("214")).
Align(lipgloss.Center).
Width(m.width)

linkStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("63")).
Align(lipgloss.Center).
Width(m.width)

menuStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("252")).
Align(lipgloss.Center).
Width(m.width)

selectedStyle := lipgloss.NewStyle().
Bold(true).
Foreground(lipgloss.Color("205")).
Background(lipgloss.Color("236")).
Align(lipgloss.Center).
Width(m.width)

var content strings.Builder
content.WriteString("\n\n")
content.WriteString(titleStyle.Render("╔═══════════════════════════════════╗"))
content.WriteString("\n")
content.WriteString(titleStyle.Render("║                                   ║"))
content.WriteString("\n")
content.WriteString(titleStyle.Render("║            N O S C L I            ║"))
content.WriteString("\n")
content.WriteString(titleStyle.Render("║                                   ║"))
content.WriteString("\n")
content.WriteString(titleStyle.Render("╚═══════════════════════════════════╝"))
content.WriteString("\n\n")
content.WriteString(versionStyle.Render("Version " + Version))
content.WriteString("\n\n")
content.WriteString(authorStyle.Render("Made by Tim from Pleb.one"))
content.WriteString("\n\n")
content.WriteString(linkStyle.Render("🔗 Pleb Signer: https://github.com/PlebOne/Pleb_Signer"))
content.WriteString("\n")
content.WriteString(linkStyle.Render("🔗 Noscli: https://github.com/PlebOne/Noscli"))
content.WriteString("\n\n\n")

if m.landingChoice == 0 {
content.WriteString(selectedStyle.Render("► Open Client"))
content.WriteString("\n")
content.WriteString(menuStyle.Render("  Settings"))
} else {
content.WriteString(menuStyle.Render("  Open Client"))
content.WriteString("\n")
content.WriteString(selectedStyle.Render("► Settings"))
}

content.WriteString("\n\n\n")
content.WriteString(versionStyle.Render("Use ↑/↓ or j/k to navigate • Enter to select • q to quit"))

return content.String()
}


func (m Model) renderSettings() string {
titleStyle := lipgloss.NewStyle().
Bold(true).
Foreground(lipgloss.Color("205")).
Padding(1, 0)

headerStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("214")).
Bold(true).
Padding(1, 0)

itemStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("252")).
Padding(0, 2)

selectedStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("205")).
Bold(true).
Background(lipgloss.Color("236")).
Padding(0, 2)

activeStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("46")).
Bold(true).
Padding(0, 2)

tabStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("241")).
Padding(0, 2)

activeTabStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("205")).
Bold(true).
Padding(0, 2)

footerStyle := lipgloss.NewStyle().
Foreground(lipgloss.Color("241")).
Padding(1, 0)

var content strings.Builder
content.WriteString(titleStyle.Render("⚙️  SETTINGS"))
content.WriteString("\n\n")

// Tab indicators
if m.settingsMenu == 0 {
content.WriteString(activeTabStyle.Render("[ Authentication ]"))
content.WriteString(tabStyle.Render("  Relays  "))
} else {
content.WriteString(tabStyle.Render("  Authentication  "))
content.WriteString(activeTabStyle.Render("[ Relays ]"))
}
content.WriteString(footerStyle.Render("  (Tab to switch)"))
content.WriteString("\n\n")

if m.settingsMenu == 0 {
// Authentication menu
content.WriteString(headerStyle.Render("Choose Authentication Method:"))
content.WriteString("\n\n")

// Pleb Signer option
authIndicator := ""
if m.authMethod == "pleb_signer" {
authIndicator = " ✓"
}
if m.settingsCursor == 0 {
content.WriteString(selectedStyle.Render(fmt.Sprintf("► Pleb Signer (DBus)%s", authIndicator)))
} else {
if m.authMethod == "pleb_signer" {
content.WriteString(activeStyle.Render(fmt.Sprintf("  Pleb Signer (DBus)%s", authIndicator)))
} else {
content.WriteString(itemStyle.Render("  Pleb Signer (DBus)"))
}
}
content.WriteString("\n")

// Nsec option
nsecIndicator := ""
if m.authMethod == "nsec" {
nsecIndicator = " ✓"
}
if m.settingsCursor == 1 {
content.WriteString(selectedStyle.Render(fmt.Sprintf("► Private Key (nsec)%s", nsecIndicator)))
} else {
if m.authMethod == "nsec" {
content.WriteString(activeStyle.Render(fmt.Sprintf("  Private Key (nsec)%s", nsecIndicator)))
} else {
content.WriteString(itemStyle.Render("  Private Key (nsec)"))
}
}
content.WriteString("\n")

if m.editingNsec {
content.WriteString("\n")
content.WriteString(headerStyle.Render("Enter your nsec key (paste supported):"))
content.WriteString("\n")
maskedKey := strings.Repeat("*", len(m.nsecKey))
content.WriteString(itemStyle.Render(fmt.Sprintf("> %s_", maskedKey)))
content.WriteString("\n")
content.WriteString(footerStyle.Render("Enter to save • Esc to cancel • Ctrl+Shift+V to paste"))
} else {
content.WriteString("\n\n")

// Show status messages
if m.statusMsg != "" {
statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Padding(0, 2)
content.WriteString(statusStyle.Render(m.statusMsg))
content.WriteString("\n")
}

if m.authMethod == "" {
content.WriteString(footerStyle.Render("⚠️  Please select an authentication method before starting client"))
content.WriteString("\n")
}
if m.authMethod != "" {
content.WriteString(footerStyle.Render("✓ Auth set! Press Tab to view/edit Relays"))
content.WriteString("\n")
}
content.WriteString(footerStyle.Render("↑/↓ nav • Enter select • Tab → Relays • Esc back"))
}

} else {
// Relays menu
content.WriteString(headerStyle.Render("Nostr Relays:"))
content.WriteString("\n\n")

for i, relay := range m.relays {
if i == m.settingsCursor {
content.WriteString(selectedStyle.Render(fmt.Sprintf("► %s", relay)))
} else {
content.WriteString(itemStyle.Render(fmt.Sprintf("  %s", relay)))
}
content.WriteString("\n")
}

if m.editingRelay {
content.WriteString("\n")
content.WriteString(headerStyle.Render("Add New Relay:"))
content.WriteString("\n")
content.WriteString(itemStyle.Render(fmt.Sprintf("> %s_", m.newRelayInput)))
content.WriteString("\n")
content.WriteString(footerStyle.Render("Enter to add • Esc to cancel"))
} else {
content.WriteString("\n")
if m.authMethod == "" {
content.WriteString(footerStyle.Render("⚠️  Set authentication method first (Tab to switch)"))
content.WriteString("\n")
content.WriteString(footerStyle.Render("↑/↓ navigate • a add relay • d/x delete • Esc/q back"))
} else {
content.WriteString(footerStyle.Render("↑/↓ navigate • a add relay • d/x delete • Enter start client • Esc/q back"))
}
}
}

return content.String()
}

func connectToPlebSignerCmd(signer *signer.PlebSigner) tea.Cmd {
return func() tea.Msg {
pubKey, err := signer.GetPublicKey()
if err != nil {
return errMsg{err}
}
return pubKeyMsg{pubKey: pubKey}
}
}

type pubKeyMsg struct {
pubKey string
}

func connectWithNsecCmd(nsecKey string) tea.Cmd {
return func() tea.Msg {
// Decode nsec to get private key
_, privKeyHex, err := nip19.Decode(nsecKey)
if err != nil {
return errMsg{fmt.Errorf("invalid nsec key: %w", err)}
}

// Get public key from private key
privKey, ok := privKeyHex.(string)
if !ok {
return errMsg{fmt.Errorf("invalid private key format")}
}

pubKey, err := nostr.GetPublicKey(privKey)
if err != nil {
return errMsg{fmt.Errorf("failed to derive public key: %w", err)}
}

return nsecAuthMsg{pubKey: pubKey, privKey: privKey}
}
}

type nsecAuthMsg struct {
pubKey  string
privKey string
}

// signEvent signs an event using either Pleb Signer or direct nsec key

// signEvent signs an event using either Pleb Signer or direct nsec key
func (m *Model) signEvent(evt *nostr.Event) error {
if m.authMethod == "nsec" && m.privKey != "" {
// Sign directly with private key
return evt.Sign(m.privKey)
} else {
// Use Pleb Signer
return m.signer.SignEvent(evt)
}
}

// unwrapGiftWrapDM unwraps a NIP-17 gift-wrapped DM (kind 1059)
func (m *Model) unwrapGiftWrapDM(giftWrapEvent nostr.Event) (string, error) {
	if m.authMethod == "nsec" && m.privKey != "" {
		// Decrypt directly with private key using NIP-44
		// Create decrypt function that NIP-59 expects
		decrypt := func(otherPubkey, ciphertext string) (string, error) {
			// Generate conversation key for NIP-44
			conversationKey, err := nip44.GenerateConversationKey(otherPubkey, m.privKey)
			if err != nil {
				return "", fmt.Errorf("failed to generate conversation key: %w", err)
			}
			// Decrypt with NIP-44
			plaintext, err := nip44.Decrypt(ciphertext, conversationKey)
			if err != nil {
				return "", fmt.Errorf("failed to decrypt: %w", err)
			}
			return plaintext, nil
		}
		
		// Use nip59.GiftUnwrap to unwrap the gift-wrapped DM
		rumor, err := nip59.GiftUnwrap(giftWrapEvent, decrypt)
		if err != nil {
			return "", fmt.Errorf("failed to unwrap gift: %w", err)
		}
		
		// The rumor contains the actual DM content
		return rumor.Content, nil
	} else {
		// Pleb Signer doesn't support NIP-44 yet
		return "", fmt.Errorf("NIP-17 DMs require nsec authentication (Pleb Signer NIP-44 support coming soon)")
	}
}

// decryptDM decrypts a DM using either Pleb Signer or direct nsec key
func (m *Model) decryptDM(ciphertext, otherPubkey string) (string, error) {
	if m.authMethod == "nsec" && m.privKey != "" {
		// Decrypt directly with private key using NIP-04
		sharedSecret, err := nip04.ComputeSharedSecret(otherPubkey, m.privKey)
		if err != nil {
			return "", err
		}
		plaintext, err := nip04.Decrypt(ciphertext, sharedSecret)
		if err != nil {
			return "", err
		}
		return plaintext, nil
	} else {
		// Use Pleb Signer (note: signature is senderPubKey first, then ciphertext)
		return m.signer.Nip04Decrypt(otherPubkey, ciphertext)
	}
}
