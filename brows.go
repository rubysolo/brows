package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/google/go-github/v48/github"
	"github.com/masterminds/semver"
	"golang.org/x/oauth2"
	tea "github.com/charmbracelet/bubbletea"
)

type release struct {
	version     string
	description string
}

type model struct {
	owner    string
	repo     string
	version  *semver.Version
	focus    string
	loaded   bool
	releases map[string]release
	gh       *github.Client
	err      error
}

func initialModel(gh *github.Client, owner, repo, version string) model {
	v, err := semver.NewVersion(version)
	if err != nil {
		log.Fatalf("Error parsing current version %v\n", err)
	}

	releases := make(map[string]release)

	return model{
		owner:    owner,
		repo:     repo,
		version:  v,
		focus:    "",
		loaded:   false,
		releases: releases,
		gh:       gh,
	}
}

func (m model) Init() tea.Cmd {
	return getReleases(m.gh, m.owner, m.repo)
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

	case errMsg:
		// There was an error. Note it in the model. And tell the runtime
		// we're done and want to quit.
		m.err = msg
		return m, tea.Quit


	case tea.KeyMsg:
		switch msg.String() {
		// These keys should exit the program.
		case "ctrl+c", "q":
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
		}
	}

	return m, nil
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
		if release, ok := m.releases[m.focus]; ok {
			s += "Release " + m.focus
			out, _ := glamour.Render(release.description, "dark")
			s += out
		} else {
			s += "error loading content from model!"
		}

	} else {
		s += "loading..."
	}

	s += "\n[q] quit [h] prev [l] next"

	return s
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

	p := tea.NewProgram(initialModel(client, owner, repo, version))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
