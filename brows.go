package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/go-github/v48/github"
	"github.com/masterminds/semver"
	"golang.org/x/oauth2"
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
)


type release struct {
	version     string
	description string
}

type model struct {
	owner     string
	repo      string
	version   *semver.Version
	focus     string
	loaded    bool
	releases  map[string]release
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
		focus:    "",
		loaded:   false,
		releases: releases,
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
				version: asString(r.TagName),
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case loadedReleases:
		// got response back from github, store in model
		m.releases = map[string]release(msg)
		m.loaded = true

		nextVersion, err := findVersion(true, m.version, m.releases)

		if err != nil {
			m.err = err
		} else {
			m.focus = nextVersion
		}

		if release, ok := m.releases[m.focus]; ok {
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
			v, err := semver.NewVersion(m.focus)

			if err != nil {
				m.err = err
			} else {
				prevVersion, err := findVersion(false, v, m.releases)

				if err != nil {
					m.err = err
				} else {
					m.focus = prevVersion
				}
			}

			if release, ok := m.releases[m.focus]; ok {
				out, _ := glamour.Render(release.description, "dark")
				m.viewport.SetContent(out)
			}

		case "right", "l":
			// navigate to next release
			v, err := semver.NewVersion(m.focus)

			if err != nil {
				m.err = err
			} else {
				nextVersion, err := findVersion(true, v, m.releases)

				if err != nil {
					m.err = err
				} else {
					m.focus = nextVersion
				}
			}

			if release, ok := m.releases[m.focus]; ok {
				out, _ := glamour.Render(release.description, "dark")
				m.viewport.SetContent(out)
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

func findVersion(greater bool, current *semver.Version, releases map[string]release) (string, error) {
	// parse versions as semver
	versions := make([]*semver.Version, len(releases))

	i := 0
	for raw := range releases {
		v, err := semver.NewVersion(raw)

		if err != nil {
			return "", err
		}

		versions[i] = v
		i++
	}

	if greater {
		sort.Sort(semver.Collection(versions))
	} else {
		sort.Sort(sort.Reverse(semver.Collection(versions)))
	}

	// return adjacent version
	for i, _ := range versions {
		if greater {
			if versions[i].GreaterThan(current) {
				return "v" + versions[i].String(), nil
			}
		} else {
			if versions[i].LessThan(current) {
				return "v" + versions[i].String(), nil
			}
		}
	}

	if greater {
		return "", fmt.Errorf("Could not find version after v%s", current)
	}

  return "", fmt.Errorf("Could not find version before v%s", current)
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

func (m model) headerView() string {
	title := titleStyle.Render(m.focus)
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(title)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line)
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
	if len(os.Args) < 3 {
		fmt.Println("Usage:")
		fmt.Println("  brows organization/repo version")
		os.Exit(1)
	}

	owner := ""
	repo := os.Args[1]
	version := os.Args[2]

	parts := strings.Split(repo, "/")
	if len(parts) == 1 {
		owner = "gruntwork-io"
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
