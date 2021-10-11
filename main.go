package main

import (
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func main() {
	repo, err := getRepoInfo()
	if err != nil {
		fmt.Println("error getting repo information", err)
		os.Exit(1)
	}
	resultCommandChan := make(chan string, 1)
	if err := tea.NewProgram(initialModel(repo.currentBranch, resultCommandChan)).Start(); err != nil {
		fmt.Printf("could not start program: %s\n", err)
		os.Exit(1)
	}

	select {
	case result := <-resultCommandChan:
		if err:= executeCommand(result); err != nil {
			fmt.Println("error executing command: ", err.Error())
			os.Exit(1)
		}
	default:
		os.Exit(0)
	}
}

func executeCommand(cmdStr string) error {
	fmt.Println("Executing command:", cmdStr)
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

var (
	focusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	cursorStyle = focusedStyle.Copy()
	noStyle     = lipgloss.NewStyle()
	lightStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Copy()
	boldStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	focusedButton = focusedStyle.Copy().Render("[ Tag and Push ]")
	blurredButton = fmt.Sprintf("[ %s ]", lightStyle.Render("Tag and Push"))
)

type model struct {
	focusIndex        int
	inputs            []textinput.Model
	cursorMode        textinput.CursorMode
	tagCommandOutChan chan string
}

func initialModel(branch string, tagCommand chan string) model {
	m := model{
		inputs: make([]textinput.Model, 2),
		tagCommandOutChan: tagCommand,
	}

	var t textinput.Model
	for i := range m.inputs {
		t = textinput.NewModel()
		t.CursorStyle = cursorStyle

		switch i {
		case 0:
			t.Focus()
			t.Placeholder = "tag"
			t.SetValue("v")
			t.CharLimit = 64
			t.PromptStyle = focusedStyle
			t.TextStyle = focusedStyle
		case 1:
			t.Placeholder = "branch"
			t.CharLimit = 200
			t.SetValue(branch)
		}

		m.inputs[i] = t
	}

	return m
}
func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit

		// Change cursor mode
		case "ctrl+r":
			m.cursorMode++
			if m.cursorMode > textinput.CursorHide {
				m.cursorMode = textinput.CursorBlink
			}
			cmds := make([]tea.Cmd, len(m.inputs))
			for i := range m.inputs {
				cmds[i] = m.inputs[i].SetCursorMode(m.cursorMode)
			}
			return m, tea.Batch(cmds...)

		// Set focus to next input
		case "tab", "shift+tab", "enter", "up", "down":
			s := msg.String()

			// Did the user press enter while the submit button was focused?
			// If so, exit.
			if s == "enter" && m.focusIndex == len(m.inputs) {
				m.tagCommandOutChan<- strings.Join(m.command(), "")
				return m, tea.Batch(tea.ExitAltScreen,tea.Quit)
			}

			// Cycle indexes
			if s == "up" || s == "shift+tab" {
				m.focusIndex--
			} else {
				m.focusIndex++
			}

			if m.focusIndex > len(m.inputs) {
				m.focusIndex = 0
			} else if m.focusIndex < 0 {
				m.focusIndex = len(m.inputs)
			}

			cmds := make([]tea.Cmd, len(m.inputs))
			for i := 0; i <= len(m.inputs)-1; i++ {
				if i == m.focusIndex {
					// Set focused state
					cmds[i] = m.inputs[i].Focus()
					m.inputs[i].PromptStyle = focusedStyle
					m.inputs[i].TextStyle = focusedStyle
					continue
				}
				// Remove focused state
				m.inputs[i].Blur()
				m.inputs[i].PromptStyle = noStyle
				m.inputs[i].TextStyle = noStyle
			}

			return m, tea.Batch(cmds...)
		}
	}

	// Handle character input and blinking
	cmd := m.updateInputs(msg)

	return m, cmd
}

func (m *model) updateInputs(msg tea.Msg) tea.Cmd {
	var cmds = make([]tea.Cmd, len(m.inputs))

	// Only text inputs with Focus() set will respond, so it's safe to simply
	// update all of them here without any further logic.
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}

	return tea.Batch(cmds...)
}

func (m model) View() string {
	var b strings.Builder

	for i := range m.inputs {
		b.WriteString(m.inputs[i].View())
		if i < len(m.inputs)-1 {
			b.WriteRune('\n')
		}
	}

	button := &blurredButton
	if m.focusIndex == len(m.inputs) {
		button = &focusedButton
	}
	fmt.Fprintf(&b, "\n\n%s\n\n", *button)

	formatLightBold(&b, m.command()...)
	return b.String()
}

func (m model) command() []string {
	tag := m.inputs[0].Value()
	branch := m.inputs[1].Value()
	command := []string {
		`git tag -a `, tag, ` -m "source=manual,branch=`, branch, `,tag=`, tag,
		`" && git push origin `, tag,
	}
	return command
}

func formatLightBold(b *strings.Builder, s ...string) {
	for i := range s {
		if i % 2 == 0 {
			b.WriteString(lightStyle.Render(s[i]))
		} else {
			b.WriteString(boldStyle.Render(s[i]))
		}
	}
}

type repoInfo struct {
	*git.Repository
	currentBranch string
}
func getRepoInfo() (*repoInfo, error) {
	repo, err := git.PlainOpen(".")
	if err != nil {
		return nil, err
	}

	branchRefs, err := repo.Branches()
	if err != nil {
		return nil, err
	}

	headRef, err := repo.Head()
	if err != nil {
		return nil, err
	}

	var currentBranch string
	_ = branchRefs.ForEach(func(bf *plumbing.Reference) error {
		if bf.Hash() == headRef.Hash() {
			currentBranch = bf.Name().Short()
			return nil
		}
		return nil
	})

	return &repoInfo{repo, currentBranch}, nil
}
