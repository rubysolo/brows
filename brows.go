package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/go-github/v48/github"
	"github.com/masterminds/semver"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
	tea "github.com/charmbracelet/bubbletea"
)

var (
	titleStyle = lipgloss.NewStyle().
												Foreground(lipgloss.Color("#E0E0E0")).
												Background(lipgloss.Color("#0066CC"))

	tagStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Right = "├"
		return lipgloss.NewStyle().BorderStyle(b).Padding(0, 1)
	}()

	infoStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Left = "┤"
		return tagStyle.Copy().BorderStyle(b)
	}()

	hCentered = func(w int) lipgloss.Style {
		return lipgloss.NewStyle().
										Width(w).
										Align(lipgloss.Center)
	}

	screenCentered = func(w, h int) lipgloss.Style {
		return lipgloss.NewStyle().
										Width(w).
										Align(lipgloss.Center).
										Height(h).
										AlignVertical(lipgloss.Center)
	}

	focusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	releaseStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5C5C5C"))
)

type Config struct {
	DefaultOrg string `yaml:"default_org"`
}

var AppConfig *Config

const configPath = ".config/brows.yml"

func ReadConfig() {
	dirname, err := os.UserHomeDir()
	path := filepath.Join(dirname, configPath)

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	decoder.Decode(&AppConfig)
}

type release struct {
	tag         string
	description string
}

type model struct {
	owner     string
	repo      string
	version   *semver.Version
	focus     int
	loaded    bool
	releases  map[string]release
	tagList   semver.Collection
	gh        *github.Client
	spinner   spinner.Model
	viewport  viewport.Model
	viewReady bool
	err       error
}

func initialModel(gh *github.Client, owner, repo, version string) model {
	v, err := semver.NewVersion(version)
	if err != nil {
		log.Fatalf("Error parsing current version %v\n", err)
	}

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	releases := make(map[string]release)

	return model{
		owner:    owner,
		repo:     repo,
		version:  v,
		loaded:   false,
		releases: releases,
		tagList:  []*semver.Version{},
		focus:    -1,
		gh:       gh,
		spinner:  spin,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(getReleases(m.gh, m.owner, m.repo), m.spinner.Tick)
}

type loadedReleases map[string]release

type errMsg struct{ err error }
func (e errMsg) Error() string { return e.err.Error() }

func getReleases(gh *github.Client, owner, repo string) tea.Cmd {
	return func() tea.Msg {
		releaseList, _, err := gh.Repositories.ListReleases(context.Background(), owner, repo, &github.ListOptions{PerPage: 1000})

		if err != nil {
			return errMsg{err}
		}

		releases := make(map[string]release)
		for _, r := range releaseList {
			releases[asString(r.TagName)] = release{
				tag: asString(r.TagName),
				description: asString(r.Body),
			}
		}

		return loadedReleases(releases)
	}
}

func asString(s *string) string {
	if s == nil {
			temp := ""
			s = &temp
	}
	return *s
}

func sortedTags(tags []string) semver.Collection {
	count := len(tags)
	tagList := make([]*semver.Version, count)

	for i, t := range tags {
		v, err := semver.NewVersion(t)
		if err != nil {
			log.Fatalf("Error parsing current version %v\n", err)
		}

		tagList[i] = v
	}

	sort.Sort(semver.Collection(tagList))

	return tagList
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case loadedReleases:
		// got response back from github, store in model
		m.releases = map[string]release(msg)

		tags := make([]string, len(m.releases))

		i := 0
		for k := range m.releases {
			tags[i] = k
			i++
		}

		m.tagList = sortedTags(tags)
		m.loaded = true

		index, err := findTagIndex(m.version, m.tagList)

		if err != nil {
			m.err = err
		} else {
			m.focus = index
		}

		tag := m.tagList[m.focus]

		if release, ok := m.releases[tag.Original()]; ok {
			out, _ := glamour.Render(release.description, "dark")
			m.viewport.SetContent(out)
		}

	case errMsg:
		// There was an error. Note it in the model. And tell the runtime
		// we're done and want to quit.
		m.err = msg
		return m, tea.Quit


	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			// exit the program
			return m, tea.Quit

		case "left", "h":
			// navigate to previous release
			if m.focus > 0 {
				m.focus = m.focus - 1
		    tag := m.tagList[m.focus]

				if release, ok := m.releases[tag.Original()]; ok {
					out, _ := glamour.Render(release.description, "dark")
					m.viewport.SetContent(out)
				}
			}

		case "right", "l":
			// navigate to next release
			if m.focus < len(m.tagList) - 1 {
				m.focus = m.focus + 1
		    tag := m.tagList[m.focus]

				if release, ok := m.releases[tag.Original()]; ok {
					out, _ := glamour.Render(release.description, "dark")
					m.viewport.SetContent(out)
				}
			}
		}

	case tea.WindowSizeMsg:
		headerHeight := lipgloss.Height(m.headerView())
		footerHeight := lipgloss.Height(m.footerView())
		verticalMarginHeight := headerHeight + footerHeight

		if !m.viewReady {
			// Since this program is using the full size of the viewport we
			// need to wait until we've received the window dimensions before
			// we can initialize the viewport. The initial dimensions come in
			// quickly, though asynchronously, which is why we wait for them
			// here.
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = headerHeight
			m.viewReady = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}

	default:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func findTagIndex(current *semver.Version, tagList semver.Collection) (int, error) {
	// return next semver tag after current
	for i, _ := range tagList {
		if tagList[i].GreaterThan(current) {
			return i, nil
		}
	}

	return -1, fmt.Errorf("Could not find version after v%s", current)
}

func (m model) View() string {
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.bodyView(), m.footerView())
}

func (m model) Title() string {
	title := fmt.Sprintf(" %s/%s Releases", m.owner, m.repo)
	title += strings.Repeat(" ", max(0, m.viewport.Width-lipgloss.Width(title)))

	return titleStyle.Render(title)
}

func isMajor(v *semver.Version) bool {
	return v.Minor() == 0 && v.Patch() == 0 && v.Prerelease() == ""
}

func isMinor(v *semver.Version) bool {
	return v.Minor() != 0 && v.Patch() == 0 && v.Prerelease() == ""
}

func isPatch(v *semver.Version) bool {
	return v.Patch() != 0 && v.Prerelease() == ""
}

func (m model) releaseList() string {
	// TODO: take a window of tags (max length = window width)
	// add next/prev arrows if off window

	// render as major/minor/patch
	rendered := ""
	var style lipgloss.Style

	for i, t := range m.tagList {
		if i == m.focus {
			style = focusStyle
		} else {
			style = releaseStyle
		}

		switch {
		case isMajor(t):
			rendered += style.Render("▇")

		case isMinor(t):
			rendered += style.Render("▅")

		case isPatch(t):
			rendered += style.Render("▂")

		default:
			rendered += style.Render(".")
		}
	}

	// center in window
	return hCentered(m.viewport.Width).Render(rendered)
}

func (m model) headerView() string {
	version := ""
	rendered := fmt.Sprintf("\n%s\n", strings.Repeat("─", max(0, m.viewport.Width)))

	if m.focus >= 0 {
		tag := m.tagList[m.focus]
		version = tag.Original()

		tagLabel := tagStyle.Render(version)
		line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(tagLabel)))
		rendered = lipgloss.JoinHorizontal(lipgloss.Center, tagLabel, line)
	}

	return fmt.Sprintf("%s\n%s\n%s", m.Title(), m.releaseList(), rendered)
}

func (m model) bodyView() string {
	if m.loaded {
		return m.viewport.View()
	} else {
		content := fmt.Sprintf("%s loading...", m.spinner.View())
		return screenCentered(m.viewport.Width, m.viewport.Height).Render(content)
	}
}

func (m model) footerView() string {
	if (m.viewport.VisibleLineCount() >= m.viewport.TotalLineCount()) {
		return fmt.Sprintf("\n%s\n", strings.Repeat("─", m.viewport.Width))
	}

	info := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(info)))
	return lipgloss.JoinHorizontal(lipgloss.Center, line, info)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  brows organization/repo [version]")
		os.Exit(1)
	}

	version := "0.0.0"

	owner := ""
	repo := os.Args[1]

	if len(os.Args) > 2 {
		version = os.Args[2]
	}

	parts := strings.Split(repo, "/")
	if len(parts) == 1 {
		ReadConfig()
		if AppConfig == nil {
			fmt.Println("No organization specified, and no default organization configured.")
			os.Exit(1)
		}
		owner = AppConfig.DefaultOrg
	} else {
		owner = parts[0]
		repo = parts[1]
	}

	token := os.Getenv("GITHUB_OAUTH_TOKEN")
	if token == "" {
		log.Fatal("no GITHUB_OAUTH_TOKEN provided.")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	p := tea.NewProgram(initialModel(client, owner, repo, version), tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
