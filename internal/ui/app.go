package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	json "github.com/goccy/go-json"
	"github.com/mrpir/hifi-tui/internal/api"
	"github.com/mrpir/hifi-tui/internal/cache"
	"github.com/mrpir/hifi-tui/internal/config"
	"github.com/mrpir/hifi-tui/internal/download"
	"github.com/mrpir/hifi-tui/internal/logger"
	"github.com/mrpir/hifi-tui/internal/models"
)

// ModalType identifies the active modal.
type ModalType int

const (
	ModalNone ModalType = iota
	ModalHelp
	ModalContextMenu
	ModalSettings
	ModalAlbumPopup
	ModalArtistPopup
	ModalTrackPopup
)

// AppModel is the root Bubble Tea model.
type AppModel struct {
	// Layout
	width  int
	height int

	// Components
	searchBar   SearchBarModel
	navPanel    NavPanelModel
	listPanel   ListPanelModel
	detailPanel DetailPanelModel
	statusBar   StatusBarModel

	// Modals
	activeModal    ModalType
	helpScreen     HelpScreenModel
	contextMenu    ContextMenuModel
	settingsScreen SettingsScreenModel
	albumPopup     AlbumPopupModel
	artistPopup    ArtistPopupModel
	trackPopup     TrackPopupModel

	// Navigation state
	state       models.NavState
	focusPanel  int // 0=nav, 1=list, 2=detail
	searchCache map[models.ViewMode]searchCacheEntry

	// Services
	api         *api.Client
	downloader  *download.Downloader
	settings    config.Settings
	searchStore *cache.SearchCache
	cancelCtx   context.Context
	cancelFunc  context.CancelFunc

	// Download queue
	queue     []models.DownloadItem
	dlTicker  bool // whether download tick is running
	keys      KeyMap
}

type searchCacheEntry struct {
	items interface{}
	query string
}

// NewApp creates the root application model.
func NewApp(settings config.Settings, apiClient *api.Client, downloader *download.Downloader, searchStore *cache.SearchCache) AppModel {
	ctx, cancel := context.WithCancel(context.Background())

	m := AppModel{
		searchBar:   NewSearchBar(),
		navPanel:    NewNavPanel(),
		listPanel:   NewListPanel(),
		detailPanel: NewDetailPanel(),
		statusBar:   NewStatusBar(),
		state:       models.NewNavState(),
		searchCache: make(map[models.ViewMode]searchCacheEntry),
		api:         apiClient,
		downloader:  downloader,
		settings:    settings,
		searchStore: searchStore,
		cancelCtx:   ctx,
		cancelFunc:  cancel,
		keys:        DefaultKeyMap(),
	}

	// Load last query if available
	if settings.LastQuery != "" {
		m.searchBar.SetValue(settings.LastQuery)
	}

	return m
}

// Init returns the initial command.
func (m AppModel) Init() tea.Cmd {
	// If there was a last query, search automatically
	if m.settings.LastQuery != "" {
		return m.doSearch(m.settings.LastQuery)
	}
	return nil
}

// Update handles messages.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case tea.KeyMsg:
		// Modal handling first
		if m.activeModal != ModalNone {
			return m.updateModal(msg)
		}

		// Search bar handling
		if m.searchBar.Visible() && m.searchBar.Focused() {
			if msg.String() == "esc" {
				m.searchBar.Hide()
				return m, nil
			}
			if cmd := m.searchBar.CheckSubmit(msg); cmd != nil {
				return m, cmd
			}
			cmd := m.searchBar.Update(msg)
			return m, cmd
		}

		// Global key bindings
		return m.handleKeyPress(msg)

	case SubmitMsg:
		return m, m.doSearch(msg.Query)

	case SearchResultMsg:
		return m.handleSearchResult(msg), nil

	case AlbumDetailMsg:
		return m.handleAlbumDetail(msg), nil

	case ArtistDetailMsg:
		return m.handleArtistDetail(msg), nil

	case DownloadTickMsg:
		m.detailPanel.ShowDownloadProgress(msg.Snap)
		m.updateStatusDownload(msg.Snap)
		if msg.Snap.Status == "completed" || msg.Snap.Status == "idle" {
			m.dlTicker = false
			m.updateQueueStatuses()
			return m, nil
		}
		return m, m.tickDownload()

	case DownloadCompleteMsg:
		m.dlTicker = false
		m.updateQueueStatuses()
		m.statusBar.UpdateView(m.state.Mode.String(), fmt.Sprintf("Done: %d downloaded, %d failed", msg.Total-msg.Failed, msg.Failed))
		return m, nil

	case SettingsSavedMsg:
		m.settings = msg.Settings
		m.api.SetQuality(msg.Settings.Quality)
		m.downloader.UpdateSettings(msg.Settings.DownloadDir, msg.Settings.Quality, msg.Settings.ParallelCount)
		m.activeModal = ModalNone
		return m, nil

	case SettingsClosedMsg:
		m.activeModal = ModalNone
		return m, nil

	case ErrorMsg:
		m.statusBar.UpdateView(m.state.Mode.String(), "Error: "+msg.Err.Error())
		return m, nil
	}

	// Forward to list panel for table navigation
	cmd := m.listPanel.Update(msg)
	cmds = append(cmds, cmd)

	// Update detail panel based on cursor
	m.updateDetail()

	return m, tea.Batch(cmds...)
}

// View renders the full UI.
func (m AppModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Build main layout
	searchView := m.searchBar.View()

	mainHeight := m.height - m.searchBar.Height() - 1 // status bar
	if mainHeight < 5 {
		mainHeight = 5
	}

	listWidth := m.width - 22 - 34 // nav + detail widths
	if listWidth < 20 {
		listWidth = 20
	}

	m.navPanel.SetHeight(mainHeight)
	m.detailPanel.SetHeight(mainHeight)
	m.listPanel.SetSize(listWidth, mainHeight)

	nav := m.navPanel.View()
	list := m.listPanel.View()
	detail := m.detailPanel.View()

	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, nav, list, detail)
	statusView := m.statusBar.View()

	fullView := ""
	if searchView != "" {
		fullView = lipgloss.JoinVertical(lipgloss.Left, searchView, mainRow, statusView)
	} else {
		fullView = lipgloss.JoinVertical(lipgloss.Left, mainRow, statusView)
	}

	// Overlay modal if active
	if m.activeModal != ModalNone {
		modal := m.renderModal()
		return m.overlay(fullView, modal)
	}

	return fullView
}

// handleKeyPress processes non-modal, non-search key presses.
func (m AppModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.cancelFunc()
		if m.searchStore != nil {
			m.searchStore.Save()
		}
		return m, tea.Quit

	case key.Matches(msg, m.keys.Search):
		m.searchBar.Show()
		return m, nil

	case key.Matches(msg, m.keys.Help):
		m.activeModal = ModalHelp
		m.helpScreen = NewHelpScreen(m.width, m.height)
		return m, nil

	case key.Matches(msg, m.keys.Settings):
		m.activeModal = ModalSettings
		m.settingsScreen = NewSettingsScreen(m.settings, m.width, m.height)
		return m, nil

	case key.Matches(msg, m.keys.Menu):
		if m.listPanel.HasItems() {
			m.activeModal = ModalContextMenu
			m.contextMenu = NewContextMenu(m.state.Mode)
			return m, nil
		}

	case key.Matches(msg, m.keys.Download):
		return m, m.startDownload()

	case key.Matches(msg, m.keys.ToggleSelect):
		id := m.listPanel.GetCurrentItemID()
		if id != "" {
			if m.state.SelectedIDs[id] {
				delete(m.state.SelectedIDs, id)
			} else {
				m.state.SelectedIDs[id] = true
			}
			m.listPanel.SetSelectedIDs(m.state.SelectedIDs)
			m.listPanel.RefreshSelection()
			m.updateStatusInfo()
		}
		return m, nil

	case key.Matches(msg, m.keys.SelectAll):
		for _, id := range m.listPanel.GetAllIDs() {
			m.state.SelectedIDs[id] = true
		}
		m.listPanel.SetSelectedIDs(m.state.SelectedIDs)
		m.listPanel.RefreshSelection()
		m.updateStatusInfo()
		return m, nil

	case key.Matches(msg, m.keys.ClearSelection):
		m.state.SelectedIDs = make(map[string]bool)
		m.listPanel.SetSelectedIDs(m.state.SelectedIDs)
		m.listPanel.RefreshSelection()
		m.updateStatusInfo()
		return m, nil

	case key.Matches(msg, m.keys.Back):
		if len(m.state.History) > 0 {
			last := m.state.History[len(m.state.History)-1]
			m.state.History = m.state.History[:len(m.state.History)-1]
			m.state.Mode = last.Mode
			m.state.SearchQuery = last.Query
			m.state.Breadcrumb = last.Breadcrumb
			m.navPanel.SetActive(m.state.Mode)
			m.restoreItems(last.Items)
			m.updateStatusInfo()
		}
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		return m.handleEnter()

	case key.Matches(msg, m.keys.Tab):
		m.focusPanel = (m.focusPanel + 1) % 3
		m.updateFocus()
		return m, nil

	case key.Matches(msg, m.keys.ShiftTab):
		m.focusPanel = (m.focusPanel + 2) % 3
		m.updateFocus()
		return m, nil

	case key.Matches(msg, m.keys.CancelJob):
		// Cancel current download context
		if m.state.Mode == models.ViewQueue {
			m.cancelFunc()
			ctx, cancel := context.WithCancel(context.Background())
			m.cancelCtx = ctx
			m.cancelFunc = cancel
		}
		return m, nil
	}

	// Nav shortcuts: number keys for mode switching
	switch msg.String() {
	case "1":
		return m.switchMode(models.ViewTracks), nil
	case "2":
		return m.switchMode(models.ViewAlbums), nil
	case "3":
		return m.switchMode(models.ViewArtists), nil
	case "4":
		return m.switchMode(models.ViewQueue), nil
	}

	// Forward to list panel
	cmd := m.listPanel.Update(msg)
	m.updateDetail()
	return m, cmd
}

// updateModal forwards key events to the active modal.
func (m AppModel) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.activeModal {
	case ModalHelp:
		if msg.String() == "esc" || msg.String() == "?" {
			m.activeModal = ModalNone
		}
		return m, nil

	case ModalContextMenu:
		switch msg.String() {
		case "esc":
			m.activeModal = ModalNone
		case "up", "k":
			m.contextMenu.Up()
		case "down", "j":
			m.contextMenu.Down()
		case "enter":
			action := m.contextMenu.Selected()
			m.activeModal = ModalNone
			return m, m.handleContextAction(action)
		}
		return m, nil

	case ModalSettings:
		cmd := m.settingsScreen.Update(msg)
		if msg.String() == "esc" {
			m.activeModal = ModalNone
			return m, nil
		}
		return m, cmd

	case ModalAlbumPopup:
		if msg.String() == "esc" {
			m.activeModal = ModalNone
			return m, nil
		}
		if msg.String() == "d" {
			m.activeModal = ModalNone
			return m, m.downloadAlbum(m.albumPopup.Album())
		}
		cmd := m.albumPopup.Update(msg)
		return m, cmd

	case ModalArtistPopup:
		if msg.String() == "esc" {
			m.activeModal = ModalNone
			return m, nil
		}
		if msg.String() == "d" {
			m.activeModal = ModalNone
			return m, m.downloadArtist(m.artistPopup.Artist())
		}
		cmd := m.artistPopup.Update(msg)
		return m, cmd

	case ModalTrackPopup:
		if msg.String() == "esc" {
			m.activeModal = ModalNone
			return m, nil
		}
		if msg.String() == "d" {
			m.activeModal = ModalNone
			return m, m.downloadTracks([]models.Track{m.trackPopup.Track()})
		}
		return m, nil
	}

	return m, nil
}

// renderModal returns the view of the active modal.
func (m AppModel) renderModal() string {
	switch m.activeModal {
	case ModalHelp:
		return m.helpScreen.View()
	case ModalContextMenu:
		return m.contextMenu.View()
	case ModalSettings:
		return m.settingsScreen.View()
	case ModalAlbumPopup:
		return m.albumPopup.View()
	case ModalArtistPopup:
		return m.artistPopup.View()
	case ModalTrackPopup:
		return m.trackPopup.View()
	}
	return ""
}

// overlay centers a modal on top of the background.
func (m AppModel) overlay(bg, modal string) string {
	// Center the modal
	modalWidth := lipgloss.Width(modal)
	modalHeight := lipgloss.Height(modal)
	x := (m.width - modalWidth) / 2
	y := (m.height - modalHeight) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	placed := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal,
		lipgloss.WithWhitespaceBackground(ColorBg),
	)
	_ = bg // background is replaced by overlay
	return placed
}

// layout recalculates component sizes.
func (m *AppModel) layout() {
	m.searchBar.SetWidth(m.width)
	m.statusBar.SetWidth(m.width)
}

func (m *AppModel) updateFocus() {
	m.listPanel.Blur()
	switch m.focusPanel {
	case 1:
		m.listPanel.Focus()
	}
}

// doSearch performs a search across all modes.
func (m AppModel) doSearch(query string) tea.Cmd {
	// Save last query
	m.settings.LastQuery = query
	go config.Save(m.settings)

	return tea.Batch(
		m.searchTracks(query),
		m.searchAlbums(query),
		m.searchArtists(query),
	)
}

func (m AppModel) searchTracks(query string) tea.Cmd {
	return func() tea.Msg {
		// Check cache first
		cacheKey := cache.Key("tracks", query, 50)
		if m.searchStore != nil {
			if cached := m.searchStore.Get(cacheKey); cached != nil {
				var tracks []models.Track
				if err := json.Unmarshal(cached, &tracks); err == nil {
					return SearchResultMsg{Mode: models.ViewTracks, Items: tracks, Query: query}
				}
			}
		}

		tracks, err := m.api.SearchTracks(context.Background(), query, 50)
		if err != nil {
			return SearchResultMsg{Mode: models.ViewTracks, Err: err, Query: query}
		}

		// Cache result
		if m.searchStore != nil {
			if data, err := json.Marshal(tracks); err == nil {
				m.searchStore.Set(cacheKey, data)
			}
		}

		return SearchResultMsg{Mode: models.ViewTracks, Items: tracks, Query: query}
	}
}

func (m AppModel) searchAlbums(query string) tea.Cmd {
	return func() tea.Msg {
		cacheKey := cache.Key("albums", query, 50)
		if m.searchStore != nil {
			if cached := m.searchStore.Get(cacheKey); cached != nil {
				var albums []models.Album
				if err := json.Unmarshal(cached, &albums); err == nil {
					return SearchResultMsg{Mode: models.ViewAlbums, Items: albums, Query: query}
				}
			}
		}

		albums, err := m.api.SearchAlbums(context.Background(), query, 50)
		if err != nil {
			return SearchResultMsg{Mode: models.ViewAlbums, Err: err, Query: query}
		}

		if m.searchStore != nil {
			if data, err := json.Marshal(albums); err == nil {
				m.searchStore.Set(cacheKey, data)
			}
		}

		return SearchResultMsg{Mode: models.ViewAlbums, Items: albums, Query: query}
	}
}

func (m AppModel) searchArtists(query string) tea.Cmd {
	return func() tea.Msg {
		cacheKey := cache.Key("artists", query, 20)
		if m.searchStore != nil {
			if cached := m.searchStore.Get(cacheKey); cached != nil {
				var artists []models.Artist
				if err := json.Unmarshal(cached, &artists); err == nil {
					return SearchResultMsg{Mode: models.ViewArtists, Items: artists, Query: query}
				}
			}
		}

		artists, err := m.api.SearchArtists(context.Background(), query, 20)
		if err != nil {
			return SearchResultMsg{Mode: models.ViewArtists, Err: err, Query: query}
		}

		if m.searchStore != nil {
			if data, err := json.Marshal(artists); err == nil {
				m.searchStore.Set(cacheKey, data)
			}
		}

		return SearchResultMsg{Mode: models.ViewArtists, Items: artists, Query: query}
	}
}

func (m *AppModel) handleSearchResult(msg SearchResultMsg) *AppModel {
	if msg.Err != nil {
		logger.Log.Warn("search error", "mode", msg.Mode, "err", msg.Err)
		return m
	}

	m.searchCache[msg.Mode] = searchCacheEntry{items: msg.Items, query: msg.Query}

	switch items := msg.Items.(type) {
	case []models.Track:
		m.navPanel.SetCounts(len(items), m.navPanel.albumCount, m.navPanel.artistCount)
		if m.state.Mode == models.ViewTracks {
			m.listPanel.SetTracks(items)
			m.state.SelectedIDs = make(map[string]bool)
			m.listPanel.SetSelectedIDs(m.state.SelectedIDs)
		}
	case []models.Album:
		m.navPanel.SetCounts(m.navPanel.trackCount, len(items), m.navPanel.artistCount)
		if m.state.Mode == models.ViewAlbums {
			m.listPanel.SetAlbums(items)
			m.state.SelectedIDs = make(map[string]bool)
			m.listPanel.SetSelectedIDs(m.state.SelectedIDs)
		}
	case []models.Artist:
		m.navPanel.SetCounts(m.navPanel.trackCount, m.navPanel.albumCount, len(items))
		if m.state.Mode == models.ViewArtists {
			m.listPanel.SetArtists(items)
			m.state.SelectedIDs = make(map[string]bool)
			m.listPanel.SetSelectedIDs(m.state.SelectedIDs)
		}
	}

	m.state.SearchQuery = msg.Query
	m.updateStatusInfo()
	return m
}

func (m *AppModel) handleAlbumDetail(msg AlbumDetailMsg) *AppModel {
	if msg.Err != nil {
		m.statusBar.UpdateView(m.state.Mode.String(), "Error: "+msg.Err.Error())
		return m
	}
	m.activeModal = ModalAlbumPopup
	m.albumPopup = NewAlbumPopup(msg.Album, msg.Tracks, m.width, m.height)
	return m
}

func (m *AppModel) handleArtistDetail(msg ArtistDetailMsg) *AppModel {
	if msg.Err != nil {
		m.statusBar.UpdateView(m.state.Mode.String(), "Error: "+msg.Err.Error())
		return m
	}
	m.activeModal = ModalArtistPopup
	m.artistPopup = NewArtistPopup(msg.Artist, msg.Albums, m.width, m.height)
	return m
}

func (m AppModel) handleEnter() (tea.Model, tea.Cmd) {
	if !m.listPanel.HasItems() {
		return m, nil
	}

	idx := m.listPanel.CursorIndex()
	switch m.state.Mode {
	case models.ViewAlbums:
		if albums, ok := m.listPanel.Items().([]models.Album); ok && idx < len(albums) {
			album := albums[idx]
			return m, func() tea.Msg {
				a, tracks, err := m.api.GetAlbumTracks(context.Background(), album.ID)
				if err != nil {
					return AlbumDetailMsg{Err: err}
				}
				return AlbumDetailMsg{Album: a, Tracks: tracks}
			}
		}
	case models.ViewArtists:
		if artists, ok := m.listPanel.Items().([]models.Artist); ok && idx < len(artists) {
			artist := artists[idx]
			return m, func() tea.Msg {
				a, albums, err := m.api.GetArtistAlbums(context.Background(), artist.ID)
				if err != nil {
					return ArtistDetailMsg{Err: err}
				}
				return ArtistDetailMsg{Artist: a, Albums: albums}
			}
		}
	case models.ViewTracks:
		if tracks, ok := m.listPanel.Items().([]models.Track); ok && idx < len(tracks) {
			track := tracks[idx]
			m.activeModal = ModalTrackPopup
			m.trackPopup = NewTrackPopup(track, m.width)
			return m, nil
		}
	}

	return m, nil
}

func (m AppModel) switchMode(mode models.ViewMode) AppModel {
	m.state.Mode = mode
	m.navPanel.SetActive(mode)
	m.state.SelectedIDs = make(map[string]bool)

	// Restore cached items for this mode
	if cached, ok := m.searchCache[mode]; ok {
		m.restoreItems(cached.items)
	} else {
		switch mode {
		case models.ViewTracks:
			m.listPanel.SetTracks(nil)
		case models.ViewAlbums:
			m.listPanel.SetAlbums(nil)
		case models.ViewArtists:
			m.listPanel.SetArtists(nil)
		case models.ViewQueue:
			m.listPanel.SetQueue(m.queue)
		}
	}

	m.updateStatusInfo()
	return m
}

func (m *AppModel) restoreItems(items interface{}) {
	switch v := items.(type) {
	case []models.Track:
		m.listPanel.SetTracks(v)
	case []models.Album:
		m.listPanel.SetAlbums(v)
	case []models.Artist:
		m.listPanel.SetArtists(v)
	case []models.DownloadItem:
		m.listPanel.SetQueue(v)
	}
}

// updateDetail refreshes the detail panel based on the current cursor position.
func (m *AppModel) updateDetail() {
	if !m.listPanel.HasItems() {
		m.detailPanel.Clear()
		return
	}

	idx := m.listPanel.CursorIndex()
	switch m.state.Mode {
	case models.ViewTracks:
		if tracks, ok := m.listPanel.Items().([]models.Track); ok && idx < len(tracks) {
			m.detailPanel.ShowTrack(tracks[idx])
		}
	case models.ViewAlbums:
		if albums, ok := m.listPanel.Items().([]models.Album); ok && idx < len(albums) {
			m.detailPanel.ShowAlbum(albums[idx])
		}
	case models.ViewArtists:
		if artists, ok := m.listPanel.Items().([]models.Artist); ok && idx < len(artists) {
			m.detailPanel.ShowArtist(artists[idx])
		}
	case models.ViewQueue:
		if items, ok := m.listPanel.Items().([]models.DownloadItem); ok && idx < len(items) {
			m.detailPanel.ShowDownload(items[idx])
		}
	}
}

func (m *AppModel) updateStatusInfo() {
	count := m.listPanel.ItemCount()
	selected := len(m.state.SelectedIDs)
	info := fmt.Sprintf("%d items", count)
	if selected > 0 {
		info += fmt.Sprintf(" (%d selected)", selected)
	}

	viewName := m.state.Mode.String()
	if m.state.Breadcrumb != "" {
		viewName += " > " + m.state.Breadcrumb
	}

	m.statusBar.UpdateView(viewName, info)
}

func (m *AppModel) updateStatusDownload(snap download.Snapshot) {
	info := ""
	if snap.TracksTotal > 0 {
		info += fmt.Sprintf("[%d/%d] ", snap.TracksDone, snap.TracksTotal)
	}
	if snap.TrackArtist != "" && snap.TrackTitle != "" {
		info += snap.TrackArtist + " - " + snap.TrackTitle + "  "
	}
	info += fmt.Sprintf("%.0f%%", snap.Percent)
	if snap.SpeedStr != "" {
		info += "  " + snap.SpeedStr
	}
	if snap.ETAStr != "" {
		info += "  ETA " + snap.ETAStr
	}

	m.statusBar.UpdateView("Downloading", info)
}

func (m *AppModel) updateQueueStatuses() {
	for i := range m.queue {
		if m.queue[i].Status == "processing" {
			if m.queue[i].Failed > 0 && m.queue[i].Failed < m.queue[i].Total {
				m.queue[i].Status = "partial"
			} else if m.queue[i].Progress >= m.queue[i].Total {
				m.queue[i].Status = "completed"
			}
		}
	}
	if m.state.Mode == models.ViewQueue {
		m.listPanel.SetQueue(m.queue)
	}
	m.navPanel.SetQueueCount(len(m.queue))
}

// startDownload initiates downloads for selected or focused items.
func (m AppModel) startDownload() tea.Cmd {
	switch m.state.Mode {
	case models.ViewTracks:
		tracks := m.getSelectedTracks()
		if len(tracks) == 0 {
			return nil
		}
		return m.downloadTracks(tracks)

	case models.ViewAlbums:
		albums := m.getSelectedAlbums()
		if len(albums) == 0 {
			return nil
		}
		var cmds []tea.Cmd
		for _, album := range albums {
			cmds = append(cmds, m.downloadAlbum(album))
		}
		return tea.Batch(cmds...)

	case models.ViewArtists:
		artists := m.getSelectedArtists()
		if len(artists) == 0 {
			return nil
		}
		var cmds []tea.Cmd
		for _, artist := range artists {
			cmds = append(cmds, m.downloadArtist(artist))
		}
		return tea.Batch(cmds...)
	}
	return nil
}

func (m *AppModel) downloadTracks(tracks []models.Track) tea.Cmd {
	item := &models.DownloadItem{
		Name:     tracks[0].Title,
		ItemType: "Track",
		SourceID: tracks[0].ID,
		Artist:   tracks[0].Artist,
		Status:   "processing",
		Total:    len(tracks),
	}
	if len(tracks) > 1 {
		item.Name = fmt.Sprintf("%d tracks", len(tracks))
		item.ItemType = "Batch"
	}
	m.queue = append(m.queue, *item)
	m.navPanel.SetQueueCount(len(m.queue))

	idx := len(m.queue) - 1

	// Start download tick if not running
	var tickCmd tea.Cmd
	if !m.dlTicker {
		m.dlTicker = true
		tickCmd = m.tickDownload()
	}

	dlCmd := func() tea.Msg {
		results, err := m.downloader.DownloadTracks(m.cancelCtx, tracks, &m.queue[idx])
		total := len(results)
		failed := 0
		for _, r := range results {
			if r.Error != nil {
				failed++
			}
		}
		if err != nil {
			return DownloadCompleteMsg{Summary: err.Error(), Total: total, Failed: failed}
		}
		return DownloadCompleteMsg{Summary: "completed", Total: total, Failed: failed}
	}

	if tickCmd != nil {
		return tea.Batch(dlCmd, tickCmd)
	}
	return dlCmd
}

func (m *AppModel) downloadAlbum(album models.Album) tea.Cmd {
	item := &models.DownloadItem{
		Name:     album.Title,
		ItemType: "Album",
		SourceID: album.ID,
		Artist:   album.Artist,
		Status:   "processing",
	}
	m.queue = append(m.queue, *item)
	m.navPanel.SetQueueCount(len(m.queue))

	idx := len(m.queue) - 1

	var tickCmd tea.Cmd
	if !m.dlTicker {
		m.dlTicker = true
		tickCmd = m.tickDownload()
	}

	dlCmd := func() tea.Msg {
		results, err := m.downloader.DownloadAlbum(m.cancelCtx, album.ID, &m.queue[idx])
		total := len(results)
		failed := 0
		for _, r := range results {
			if r.Error != nil {
				failed++
			}
		}
		if err != nil {
			return DownloadCompleteMsg{Summary: err.Error(), Total: total, Failed: failed}
		}
		return DownloadCompleteMsg{Summary: "completed", Total: total, Failed: failed}
	}

	if tickCmd != nil {
		return tea.Batch(dlCmd, tickCmd)
	}
	return dlCmd
}

func (m *AppModel) downloadArtist(artist models.Artist) tea.Cmd {
	item := &models.DownloadItem{
		Name:     artist.Name,
		ItemType: "Artist",
		SourceID: artist.ID,
		Status:   "processing",
	}
	m.queue = append(m.queue, *item)
	m.navPanel.SetQueueCount(len(m.queue))

	idx := len(m.queue) - 1

	var tickCmd tea.Cmd
	if !m.dlTicker {
		m.dlTicker = true
		tickCmd = m.tickDownload()
	}

	dlCmd := func() tea.Msg {
		results, err := m.downloader.DownloadArtist(m.cancelCtx, artist.ID, &m.queue[idx])
		total := len(results)
		failed := 0
		for _, r := range results {
			if r.Error != nil {
				failed++
			}
		}
		if err != nil {
			return DownloadCompleteMsg{Summary: err.Error(), Total: total, Failed: failed}
		}
		return DownloadCompleteMsg{Summary: "completed", Total: total, Failed: failed}
	}

	if tickCmd != nil {
		return tea.Batch(dlCmd, tickCmd)
	}
	return dlCmd
}

func (m AppModel) tickDownload() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return DownloadTickMsg{Snap: m.downloader.Progress().Snap()}
	})
}

func (m AppModel) handleContextAction(action string) tea.Cmd {
	switch action {
	case "download":
		return m.startDownload()
	case "view_details":
		return m.handleEnterCmd()
	case "view_tracks":
		return m.handleEnterCmd()
	case "view_albums":
		return m.handleEnterCmd()
	}
	return nil
}

func (m AppModel) handleEnterCmd() tea.Cmd {
	_, cmd := m.handleEnter()
	return cmd
}

func (m *AppModel) getSelectedTracks() []models.Track {
	tracks, ok := m.listPanel.Items().([]models.Track)
	if !ok {
		return nil
	}

	if len(m.state.SelectedIDs) > 0 {
		var selected []models.Track
		for _, t := range tracks {
			if m.state.SelectedIDs[fmt.Sprintf("%d", t.ID)] {
				selected = append(selected, t)
			}
		}
		return selected
	}

	// Use focused item
	idx := m.listPanel.CursorIndex()
	if idx < len(tracks) {
		return []models.Track{tracks[idx]}
	}
	return nil
}

func (m *AppModel) getSelectedAlbums() []models.Album {
	albums, ok := m.listPanel.Items().([]models.Album)
	if !ok {
		return nil
	}

	if len(m.state.SelectedIDs) > 0 {
		var selected []models.Album
		for _, a := range albums {
			if m.state.SelectedIDs[fmt.Sprintf("%d", a.ID)] {
				selected = append(selected, a)
			}
		}
		return selected
	}

	idx := m.listPanel.CursorIndex()
	if idx < len(albums) {
		return []models.Album{albums[idx]}
	}
	return nil
}

func (m *AppModel) getSelectedArtists() []models.Artist {
	artists, ok := m.listPanel.Items().([]models.Artist)
	if !ok {
		return nil
	}

	if len(m.state.SelectedIDs) > 0 {
		var selected []models.Artist
		for _, a := range artists {
			if m.state.SelectedIDs[fmt.Sprintf("%d", a.ID)] {
				selected = append(selected, a)
			}
		}
		return selected
	}

	idx := m.listPanel.CursorIndex()
	if idx < len(artists) {
		return []models.Artist{artists[idx]}
	}
	return nil
}
