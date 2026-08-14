package jumper

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Terminal is the interface for a terminal session to a target.
type Terminal interface {
	io.ReadWriteCloser
	Resize(rows, cols uint16) error
}

// Target represents a connectable target in the bastion host.
type Target struct {
	ID      string
	Name    string
	Address string
	Info    string
}

// TargetResolver resolves available targets for the bastion host.
type TargetResolver interface {
	Resolve(ctx context.Context) ([]Target, error)
}

// TerminalDialer creates a terminal session to a given target.
type TerminalDialer interface {
	Dial(ctx context.Context, target Target) (Terminal, error)
}

// Window represents a terminal window size change.
type Window struct {
	Width  int
	Height int
}

// ---------------------------------------------------------------------------
// Entry points
// ---------------------------------------------------------------------------

// Serve runs the bastion host over explicit I/O streams (e.g. an SSH session).
func Serve(ctx context.Context, resolver TargetResolver, dialer TerminalDialer,
	stdin io.Reader, stdout io.Writer, resizeCh <-chan Window) error {

	return runLoop(ctx, resolver, dialer, stdin, stdout, resizeCh)
}

// runLoop alternates between the target selection menu and terminal sessions
// until the user quits or the context is cancelled.
func runLoop(ctx context.Context, resolver TargetResolver, dialer TerminalDialer,
	stdin io.Reader, stdout io.Writer, resizeCh <-chan Window) error {

	pump := newWindowPump(resizeCh)
	defer pump.stop()

	mux := newInputMux(ctx, stdin)

	for {
		// Menu phase.
		menuReader := mux.reader()
		outcome := runMenu(ctx, resolver, menuReader, stdout, pump)
		menuReader.Close()
		if outcome.err != nil {
			return outcome.err
		}
		if outcome.quit || outcome.target == nil {
			return nil
		}

		// Session + prompt phase: keep reconnecting to the same target
		// until the user opts to return to the list.
		for {
			sessReader := mux.reader()
			err := runSession(ctx, dialer, *outcome.target, sessReader, stdout, pump)
			sessReader.Close()

			promptReader := mux.reader()
			go func(r *muxReader) {
				timer := time.NewTimer(30 * time.Second)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-ctx.Done():
				}
				r.Close()
			}(promptReader)
			reconnect := promptAfterSession(stdout, promptReader, err)
			promptReader.Close()

			if reconnect {
				continue
			}
			// Return to the list: leave the session/prompt screen.
			_, _ = stdout.Write([]byte("\x1b[?1049l"))
			break
		}
	}
}

// windowPump forwards window size events keeping only the latest pending one,
// while remembering the last known size so it can be replayed when switching
// between the menu and a session.
type windowPump struct {
	mu       sync.Mutex
	ch       chan Window
	lastSize Window
	hasSize  bool
	done     chan struct{}
}

func newWindowPump(resizeCh <-chan Window) *windowPump {
	p := &windowPump{
		ch:   make(chan Window, 1),
		done: make(chan struct{}),
	}
	go p.loop(resizeCh)
	return p
}

func (p *windowPump) loop(resizeCh <-chan Window) {
	for {
		select {
		case <-p.done:
			return
		case w, ok := <-resizeCh:
			if !ok {
				return
			}
			p.mu.Lock()
			p.lastSize = w
			p.hasSize = true
			p.publishLocked(w)
			p.mu.Unlock()
		}
	}
}

// replay delivers the last known size into the channel so the active phase
// (menu or session) starts at the correct window size even if no resize event
// arrives on the mode switch.
func (p *windowPump) replay() {
	p.mu.Lock()
	if p.hasSize {
		p.publishLocked(p.lastSize)
	}
	p.mu.Unlock()
}

// stop signals the pump goroutine to exit so it does not leak when the run
// loop ends.
func (p *windowPump) stop() { close(p.done) }

// publishLocked writes w to the single-slot channel, replacing any pending
// value. The caller must hold p.mu so the drain-and-publish step is atomic
// with respect to other callers; this prevents the non-blocking send pattern
// from deadlocking when two goroutines race on the same slot.
func (p *windowPump) publishLocked(w Window) {
	select {
	case p.ch <- w:
	default:
		<-p.ch
		p.ch <- w
	}
}

// ---------------------------------------------------------------------------
// Selection menu
// ---------------------------------------------------------------------------

type selectionState int

const (
	stateLoading selectionState = iota
	stateReady
	stateError
)

const pageSize = 10

type tickMsg time.Time
type targetsLoadedMsg struct {
	targets []Target
}
type errMsg struct {
	err error
}

// menuOutcome is the result of a menu run.
type menuOutcome struct {
	target *Target
	quit   bool
	err    error
}

func runMenu(ctx context.Context, resolver TargetResolver,
	stdin io.Reader, stdout io.Writer, pump *windowPump) menuOutcome {

	pump.replay()

	program := tea.NewProgram(
		newMenuModel(ctx, resolver),
		tea.WithInput(stdin),
		tea.WithOutput(stdout),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case w, ok := <-pump.ch:
				if !ok {
					return
				}
				select {
				case <-stop:
					return
				default:
					program.Send(tea.WindowSizeMsg{Width: w.Width, Height: w.Height})
				}
			}
		}
	}()

	model, err := program.Run()
	close(stop)
	if err != nil {
		return menuOutcome{err: err}
	}
	m := model.(menuModel)
	return menuOutcome{
		target: m.resultTarget,
		quit:   m.resultQuit,
		err:    m.err,
	}
}

type menuModel struct {
	ctx      context.Context
	resolver TargetResolver

	// program result
	err          error
	resultTarget *Target
	resultQuit   bool

	width  int
	height int

	selState  selectionState
	targets   []Target
	filtered  []Target
	filter    []rune
	cursor    int
	page      int
	selErr    error
	startTime time.Time
	elapsed   time.Duration
}

func newMenuModel(ctx context.Context, resolver TargetResolver) menuModel {
	return menuModel{
		ctx:       ctx,
		resolver:  resolver,
		selState:  stateLoading,
		startTime: time.Now(),
		width:     80,
		height:    24,
	}
}

// Init initializes the logger.
func (m menuModel) Init() tea.Cmd {
	return tea.Batch(m.loadTargets, m.tick())
}

// Update is an exported method.
func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		// Global: quit.
		switch key {
		case "ctrl+c", "q":
			m.resultQuit = true
			return m, tea.Quit
		}

		// Refresh works in both the ready and error states.
		if key == "r" && (m.selState == stateReady || m.selState == stateError) {
			return m.refresh()
		}

		// Navigation/filter input only applies once targets are loaded.
		if m.selState != stateReady {
			return m, nil
		}

		switch key {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m = m.clampPage()
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m = m.clampPage()
			}
		case "left", "pgup":
			if m.page > 0 {
				m.page--
				m.cursor = m.pageStart()
			}
		case "right", "pgdown":
			if m.page < m.totalPages()-1 {
				m.page++
				m.cursor = m.pageStart()
			}
		case "enter", " ":
			if len(m.filtered) == 0 {
				return m, nil
			}
			m.resultTarget = &m.filtered[m.cursor]
			return m, tea.Quit
		case "esc":
			if len(m.filter) > 0 {
				m.filter = nil
				m = m.applyFilter()
			}
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m = m.applyFilter()
			}
		default:
			if len(key) == 1 {
				if r := rune(key[0]); r >= 32 && r <= 126 {
					m.filter = append(m.filter, r)
					m = m.applyFilter()
				}
			}
		}

	case tickMsg:
		if m.selState == stateLoading {
			m.elapsed = time.Since(m.startTime).Round(time.Second)
			return m, m.tick()
		}

	case targetsLoadedMsg:
		m.targets = msg.targets
		m.filtered = make([]Target, len(m.targets))
		copy(m.filtered, m.targets)
		if len(m.targets) == 0 {
			m.selState = stateError
			m.selErr = fmt.Errorf("no targets available")
		} else {
			m.selState = stateReady
		}
		return m, nil

	case errMsg:
		m.selState = stateError
		m.selErr = msg.err
		return m, nil
	}

	return m, nil
}

// refresh re-enters the loading state and re-issues the target fetch +
// elapsed-time tick commands.
func (m menuModel) refresh() (tea.Model, tea.Cmd) {
	m.selState = stateLoading
	m.startTime = time.Now()
	m.selErr = nil
	return m, tea.Batch(m.loadTargets, m.tick())
}

func (m menuModel) applyFilter() menuModel {
	m.filtered = nil
	raw := string(m.filter)
	for _, t := range m.targets {
		if raw == "" || strings.Contains(strings.ToLower(t.Name), strings.ToLower(raw)) {
			m.filtered = append(m.filtered, t)
		}
	}
	if len(m.filtered) == 0 {
		m.cursor = 0
		m.page = 0
		return m
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	return m.clampPage()
}

func (m menuModel) clampPage() menuModel {
	if len(m.filtered) == 0 {
		m.page = 0
		return m
	}
	maxPage := (len(m.filtered) - 1) / pageSize
	if m.page > maxPage {
		m.page = maxPage
	}
	pageStart := m.page * pageSize
	if m.cursor < pageStart {
		m.page = m.cursor / pageSize
	}
	if m.cursor >= pageStart+pageSize {
		m.page = m.cursor / pageSize
	}
	return m
}

func (m menuModel) pageStart() int {
	return m.page * pageSize
}

func (m menuModel) pageEnd() int {
	end := m.pageStart() + pageSize
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	return end
}

func (m menuModel) totalPages() int {
	if len(m.filtered) == 0 {
		return 1
	}
	return (len(m.filtered)-1)/pageSize + 1
}

func (m menuModel) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m menuModel) loadTargets() tea.Msg {
	targets, err := m.resolver.Resolve(m.ctx)
	if err != nil {
		return errMsg{err}
	}
	return targetsLoadedMsg{targets}
}

// Static lipgloss styles and constants reused across View renders.
var (
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	accentStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	yellowStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	redStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	itemPadStyle     = lipgloss.NewStyle().Padding(0, 1)
	selectedPadStyle = lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("236"))
	cursorGlyph      = accentStyle.Render("▸")
	spacer           = lipgloss.NewStyle().Render(" ")

	spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	menuHelpText  = "↑/↓ j/k: nav  •  ←/→: page  •  enter: connect  •  esc: clear  •  r: refresh  •  q: quit"
	errorHelpText = "r: retry  •  q: quit"
)

// View is an exported method.
func (m menuModel) View() string {
	switch m.selState {
	case stateLoading:
		return m.styleLoading()
	case stateError:
		return m.styleError()
	default:
		return m.styleList()
	}
}

func (m menuModel) styleLoading() string {
	title := titleStyle.Render(" RLark Bastion Host ")

	frame := int(m.elapsed.Seconds()) % len(spinnerFrames)
	loading := dimStyle.Render(fmt.Sprintf("%s  Loading targets...  (%s)",
		string(spinnerFrames[frame]), m.elapsed))

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(lipgloss.JoinVertical(lipgloss.Center, title, "", loading))
}

func (m menuModel) styleError() string {
	title := titleStyle.Render(" RLark Bastion Host ")

	errBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(0, 2).
		Render(redStyle.Render(fmt.Sprintf("Error: %v", m.selErr)))

	help := dimStyle.Render(errorHelpText)

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(lipgloss.JoinVertical(lipgloss.Center, title, "", errBox, "", help))
}

func (m menuModel) styleList() string {
	title := titleStyle.Padding(0, 1).Render(" RLark Bastion Host ")
	filterBar := m.renderFilterBar()
	body := m.renderListBody()
	detail := m.renderDetail()
	help := dimStyle.Render(menuHelpText)

	return lipgloss.NewStyle().
		Width(m.width).
		Padding(1, 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, "", filterBar, "", body, detail, "", help))
}

func (m menuModel) renderFilterBar() string {
	filterValue := string(m.filter)
	if filterValue == "" {
		filterValue = dimStyle.Render("type to filter...")
	} else {
		filterValue = yellowStyle.Render(filterValue)
	}
	count := dimStyle.Render(fmt.Sprintf("(%d/%d)", len(m.filtered), len(m.targets)))
	display := lipgloss.JoinHorizontal(lipgloss.Left,
		accentStyle.Render("search"), spacer, filterValue, spacer, count,
	)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(m.width - 6).
		Render(display)
}

func (m menuModel) renderListBody() string {
	subtitle := dimStyle.Render(fmt.Sprintf("  Page %d/%d", m.page+1, m.totalPages()))

	var items []string
	for i := m.pageStart(); i < m.pageEnd(); i++ {
		t := m.filtered[i]
		isCursor := i == m.cursor

		glyph := " "
		if isCursor {
			glyph = cursorGlyph
		}

		var name string
		if isCursor {
			name = selectedPadStyle.Render(yellowStyle.Bold(true).Render(t.Name))
		} else {
			name = itemPadStyle.Render(t.Name)
		}

		line := glyph + " " + name
		if t.Info != "" {
			line += dimStyle.Render("  " + t.Info)
		}
		items = append(items, line)
	}

	if len(items) == 0 {
		items = append(items, dimStyle.Padding(0, 1).Render("no targets match filter"))
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		append([]string{subtitle}, items...)...)
}

func (m menuModel) renderDetail() string {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return ""
	}
	t := m.filtered[m.cursor]
	lines := []string{
		fmt.Sprintf("ID:      %s", t.ID),
		fmt.Sprintf("Name:    %s", t.Name),
	}
	if t.Address != "" {
		lines = append(lines, fmt.Sprintf("Address: %s", t.Address))
	}
	if t.Info != "" {
		lines = append(lines, fmt.Sprintf("Info:    %s", t.Info))
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(0, 1).
		Width(m.width - 6).
		Render(strings.Join(lines, "\n"))
	return "\n" + box
}
