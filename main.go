package main

// An example Bubble Tea server. This will put an ssh session into alt screen
// and continually print up to date terminal information.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	cssh "golang.org/x/crypto/ssh"
)

const (
	host = "localhost"
	port = "23232"
)

func main() {
	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
			activeterm.Middleware(), // Bubble Tea apps usually require a PTY.
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Error("Could not start server", "error", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Info("Starting SSH server", "host", host, "port", port)
	go func() {
		if err = s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("Could not start server", "error", err)
			done <- nil
		}
	}()

	<-done
	log.Info("Stopping SSH server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() { cancel() }()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("Could not stop server", "error", err)
	}
}

// You can wire any Bubble Tea model up to the middleware with a function that
// handles the incoming ssh.Session. Here we just grab the terminal info and
// pass it to the new model. You can also return tea.ProgramOptions (such as
// tea.WithAltScreen) on a session by session basis.
func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	// This should never fail, as we are using the activeterm middleware.
	pty, _, _ := s.Pty()
	fmt.Print(pty.Term)
	// When running a Bubble Tea app over SSH, you shouldn't use the default
	// lipgloss.NewStyle function.
	// That function will use the color profile from the os.Stdin, which is the
	// server, not the client.
	// We provide a MakeRenderer function in the bubbletea middleware package,
	// so you can easily get the correct renderer for the current session, and
	// use it to create the styles.
	// The recommended way to use these styles is to then pass them down to
	// your Bubble Tea model.
	renderer := bubbletea.MakeRenderer(s)

	bg := "light"
	if renderer.HasDarkBackground() {
		bg = "dark"
	}

	fmt.Print(bg)
    // Generate a unique ID for the session based on username and public key fingerprint
    sessionID := generateSessionID(s)

    // Load selections for this session from persistent storage
    var selected map[int]struct{} = make(map[int]struct{})
	err := loadSessionDataByKey(sessionID, "selected", &selected)
	if err != nil {
		log.Error("Could not load session data", "error", err)
	}


    // Create the model with the loaded selections
    m := model{
        choices:  []string{"Apples", "Bananas", "Carrots", "Durians", "Eggplants"},
        selected: selected,
        cursor:   0,
        session:  s,
        sessionID: sessionID, // Pass sessionID to the model
		activeTab: "selection",
    }


	return m, []tea.ProgramOption{tea.WithAltScreen()}
}


type TransactionTypesModel struct {
	"Deposit",
	"Withdrawal",
}

type TransactionModel struct {
	name string
	amount int
	date string
	transactionType TransactionTypesModel
}

// Just a generic tea.Model to demo terminal information of ssh.
type model struct {
    choices  []string
    cursor   int
    selected map[int]struct{}
    session  ssh.Session
    sessionID string // Store the session ID to save data
	activeTab string
	transactions []TransactionModel
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

    // Is it a key press?
    case tea.KeyMsg:

        // Cool, what was the actual key pressed?
        switch msg.String() {

		case "tab":
			if m.activeTab == "selection" {
				m.activeTab = "output"
			} else {
				m.activeTab = "selection"
			}

        // These keys should exit the program.
        case "ctrl+c", "q":

            return m, tea.Quit

        // The "up" and "k" keys move the cursor up
        case "up", "k":
            if m.cursor > 0 {
                m.cursor--
            }

        // The "down" and "j" keys move the cursor down
        case "down", "j":
            if m.cursor < len(m.choices)-1 {
                m.cursor++
            }

        // The "enter" key and the spacebar (a literal space) toggle
        // the selected state for the item that the cursor is pointing at.
        case "enter", " ":
            _, ok := m.selected[m.cursor]
            if ok {
                delete(m.selected, m.cursor)
            } else {
                m.selected[m.cursor] = struct{}{}
            }

			// Update session context after a selection change
            //saveSelectionsForSession(m.sessionID, m.selected)
			saveSessionDataByKey(m.sessionID, "selected", m.selected)

        }

    }

    // Return the updated model to the Bubble Tea runtime for processing.
    // Note that we're not returning a command.
    return m, nil
}



func (m model) View() string {
	var s string
	switch m.activeTab {
		case "selection":
			// The header
			s = "What should we buy at the market?\n\n"

			// Iterate over our choices
			for i, choice := range m.choices {
		
				// Is the cursor pointing at this choice?
				cursor := " " // no cursor
				if m.cursor == i {
					cursor = ">" // cursor!
				}
		
				// Is this choice selected?
				checked := " " // not selected
				if _, ok := m.selected[i]; ok {
					checked = "x" // selected!
				}
		
				// Render the row
				s += fmt.Sprintf("%s [%s] %s\n", cursor, checked, choice)
			}
		
			// The footer
			s += "\nPress q to quit.\n"
		case "output":
			for i, choice := range m.choices {
		
				// Is the cursor pointing at this choice?
				cursor := " " // no cursor
				if m.cursor == i {
					cursor = ">" // cursor!
				}
		
				// Is this choice selected?
				checked := " " // not selected
				if _, ok := m.selected[i]; ok {
					checked = "x" // selected!
				}
		
				if (checked == "x") {
					s += fmt.Sprintf("%s [%s] %s\n", cursor, checked, choice)
				}
				// Render the row
			}
	}

	highlight := lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#FFA900"}


	activeTabBorder := lipgloss.Border{
		Top:         "─",
		Bottom:      " ",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┘",
		BottomRight: "└",
	}

	tabBorder := lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┴",
		BottomRight: "┴",
	}

	tab := lipgloss.NewStyle().
		Border(tabBorder, true).
		BorderForeground(highlight).
		Padding(0, 1)

	activeTab := tab.Border(activeTabBorder, true).Align(lipgloss.Left)

	tabGap := tab.
		BorderTop(false).
		BorderLeft(false).
		BorderRight(false)

	row := lipgloss.JoinHorizontal(
		lipgloss.Top,
		func() string {
			if m.activeTab == "selection" {
				return activeTab.Render("Selection")
			}
			return tab.Render("Selection")
		}(),
		func() string {
			if m.activeTab == "output" {
				return activeTab.Render("Output")
			}
			return tab.Render("Output")
		}(),
	)

	pty, _, _ := m.session.Pty()

	gap := tabGap.Render(strings.Repeat(" ", max(0, pty.Window.Width-lipgloss.Width(row)-2)))
	row = lipgloss.JoinHorizontal(lipgloss.Bottom, row, gap)


	return row + "\n\n" + s
}

// Function to generate a unique session ID based on SSH username and public key fingerprint
func generateSessionID(s ssh.Session) string {
    username := s.User()
    pubKey := s.PublicKey()
    
    if pubKey != nil {
        fingerprint := cssh.FingerprintSHA256(pubKey)
        return fmt.Sprintf("%s-%s", username, fingerprint)
    }

    return username  // Fallback to username if no public key
}

func loadSessionDataByKey(sessionID string, key string, v interface{}) error {
    path := getSessionDataFilePath(sessionID)
    file, err := os.Open(path)
    if err != nil {
        if os.IsNotExist(err) {
            // No session data exists yet, so initialize the value as empty based on its type
            initializeEmpty(v)
            return nil
        }

        log.Error("Could not open session data file", "error", err)
        return err
    }
    defer file.Close()

    // Decode the JSON file into a generic map[string]json.RawMessage
    var sessionData map[string]json.RawMessage
    decoder := json.NewDecoder(file)
    if err := decoder.Decode(&sessionData); err != nil {
        log.Error("Could not decode session data file", "error", err)
        return err
    }

    // Try to find the key in the session data and decode it into v
    if data, ok := sessionData[key]; ok {
        if err := json.Unmarshal(data, v); err != nil {
            log.Error("Could not unmarshal session data for key", "error", err)
            return err
        }
    } else {
        // Key not found, initialize an empty value based on the type of v
        initializeEmpty(v)
    }

    return nil
}

// InitializeEmpty sets v to an empty value based on its type using reflection
func initializeEmpty(v interface{}) {
    rv := reflect.ValueOf(v)
    if rv.Kind() != reflect.Ptr || rv.IsNil() {
        log.Error("initializeEmpty requires a non-nil pointer")
        return
    }

    // Get the actual element type (dereferencing the pointer)
    rvElem := rv.Elem()

    // Check if the value is valid and settable
    if rvElem.IsValid() && rvElem.CanSet() {
        // Create a new zero value based on the element's type and set it
        rvElem.Set(reflect.Zero(rvElem.Type()))
    }
}


// Save data for a given session by a specific key
func saveSessionDataByKey(sessionID string, key string, v interface{}) error {
    path := getSessionDataFilePath(sessionID)

    // Open existing file or create a new one
    var sessionData map[string]json.RawMessage
    file, err := os.Open(path)
    if err == nil {
        // If the file exists, read the current session data into sessionData
        defer file.Close()
        decoder := json.NewDecoder(file)
        if err := decoder.Decode(&sessionData); err != nil {
            log.Error("Could not decode existing session data", "error", err)
            sessionData = make(map[string]json.RawMessage)
        }
    } else if os.IsNotExist(err) {
        // If the file doesn't exist, start with an empty map
        sessionData = make(map[string]json.RawMessage)
    } else {
        log.Error("Could not open session data file", "error", err)
        return err
    }

    // Marshal the new data and store it in sessionData under the provided key
    data, err := json.Marshal(v)
    if err != nil {
        log.Error("Could not marshal data", "error", err)
        return err
    }
    sessionData[key] = data

    // Save the updated session data to the file
    file, err = os.Create(path)
    if err != nil {
        log.Error("Could not create session data file", "error", err)
        return err
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    if err := encoder.Encode(&sessionData); err != nil {
        log.Error("Could not save session data", "error", err)
        return err
    }

    return nil
}


// Get the file path for the session's data file based on the sessionID
func getSessionDataFilePath(sessionID string) string {
    return filepath.Join("session_data", fmt.Sprintf("%s.json", sessionID))
}

