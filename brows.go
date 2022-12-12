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
	titleStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Right = "├"
		return lipgloss.NewStyle().BorderStyle(b).Padding(0, 1)
	}()

	infoStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Left = "┤"
		return titleStyle.Copy().BorderStyle(b)
	}()

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

type tag struct {
	tag    string
	parsed *semver.Version
	prev   *tag
	next   *tag
}

type TagList []tag

func (tl TagList) Len() int {
	return len(tl)
}

func (tl TagList) Less(i, j int) bool {
	return tl[i].parsed.LessThan(tl[j].parsed)
}

func (tl TagList) Swap(i, j int) {
	tl[i], tl[j] = tl[j], tl[i]
}

type release struct {
	tag         string
	description string
}

type model struct {
	owner     string
	repo      string
	version   *semver.Version
	focus     *tag
	loaded    bool
	releases  map[string]release
	tagList   TagList
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
		tagList:  []tag{},
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

func sortedTags(tags []string) TagList {
	count := len(tags)
	tagList := make([]tag, count)

	for i, t := range tags {
		v, err := semver.NewVersion(t)
		if err != nil {
			log.Fatalf("Error parsing current version %v\n", err)
		}

		tagList[i] = tag{tag: t, parsed: v}
	}

	sort.Sort(TagList(tagList))

	for i := range tagList {
		if i > 0 {
			tagList[i].prev = &tagList[i - 1]
		}

		if i < count - 1 {
			tagList[i].next = &tagList[i + 1]
		}
	}

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

		nextTag, err := findTag(m.version, m.tagList)

		if err != nil {
			m.err = err
		} else {
			m.focus = nextTag
		}

		if release, ok := m.releases[m.focus.tag]; ok {
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
			prevTag := m.focus.prev

			if prevTag != nil {
				m.focus = prevTag

				if release, ok := m.releases[m.focus.tag]; ok {
					out, _ := glamour.Render(release.description, "dark")
					m.viewport.SetContent(out)
				}
			}

		case "right", "l":
			// navigate to next release
			nextTag := m.focus.next

			if nextTag != nil {
				m.focus = nextTag

				if release, ok := m.releases[m.focus.tag]; ok {
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

func findTag(current *semver.Version, tagList TagList) (*tag, error) {
	// return next semver tag after current
	for i, _ := range tagList {
		if tagList[i].parsed.GreaterThan(current) {
			return &tagList[i], nil
		}
	}

	return nil, fmt.Errorf("Could not find version after v%s", current)
}

func (m model) View() string {
	s := m.owner + "/" + m.repo + " Releases\n\n"

	if m.loaded {
		return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
	} else {
		s += fmt.Sprintf("\n\n   %s loading...\n\n", m.spinner.View())
	}

	s += "\n[q] quit [h] prev [l] next"

	return s
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

	for _, t := range m.tagList {
		if t.tag == m.focus.tag {
			style = focusStyle
		} else {
			style = releaseStyle
		}

		switch {
		case isMajor(t.parsed):
			rendered += style.Render("▇")

		case isMinor(t.parsed):
			rendered += style.Render("▅")

		case isPatch(t.parsed):
			rendered += style.Render("▂")

		default:
			rendered += style.Render("_")
		}
	}

	// center in window
	var centered = lipgloss.NewStyle().
    Width(m.viewport.Width).
    Align(lipgloss.Center)

	return centered.Render(rendered)
}

func (m model) headerView() string {
	version := ""
	if m.focus != nil {
		version = m.focus.tag
	}

	title := titleStyle.Render(version)
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(title)))
	rendered := lipgloss.JoinHorizontal(lipgloss.Center, title, line)

	return fmt.Sprintf("%s\n%s", m.releaseList(), rendered)
}

func (m model) footerView() string {
	if (m.viewport.VisibleLineCount() >= m.viewport.TotalLineCount()) {
	  return strings.Repeat("─", m.viewport.Width)
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
