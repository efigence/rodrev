package repl

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/glycerine/liner"
)

// DefaultHistoryFile returns the history file path in the user's home dir
func DefaultHistoryFile() string {
	home, err := os.UserHomeDir()
	if err != nil || len(home) == 0 {
		return ""
	}
	return filepath.Join(home, ".rv_query_history")
}

// TerminalSupported reports whether stdin is a terminal liner can drive
func TerminalSupported() bool { return liner.TerminalSupported() }

// termReader is a liner-backed LineReader with history and tab completion.
// This is the only part of the REPL that needs a terminal
type termReader struct {
	l           *liner.State
	historyFile string
	log         logf
}

type logf func(format string, args ...interface{})

// NewTermReader wires liner up to the session's completer and history file.
// An empty historyFile disables history persistence
func NewTermReader(s *Session, historyFile string) LineReader {
	l := liner.NewLiner()
	l.SetCtrlCAborts(true)
	l.SetTabCompletionStyle(liner.TabPrints)
	l.SetWordCompleter(func(line string, pos int) (string, []string, string) {
		return Complete(s.CompletionState(), line, pos)
	})
	t := termReader{
		l:           l,
		historyFile: historyFile,
		log:         s.log.Debugf,
	}
	t.readHistory()
	return &t
}

func (t *termReader) Prompt(prompt string) (string, error) {
	line, err := t.l.Prompt(prompt)
	switch err {
	case nil:
	case liner.ErrPromptAborted:
		return "", ErrInterrupted
	case liner.ErrNotTerminalOutput:
		return "", ErrNoTerminal
	case io.EOF:
		return "", io.EOF
	default:
		return "", err
	}
	if len(strings.TrimSpace(line)) > 0 {
		// liner drops consecutive duplicates and caps at liner.HistoryLimit
		t.l.AppendHistory(line)
	}
	return line, nil
}

func (t *termReader) Close() error {
	t.writeHistory()
	return t.l.Close()
}

func (t *termReader) readHistory() {
	if len(t.historyFile) == 0 {
		return
	}
	fd, err := os.Open(t.historyFile)
	if err != nil {
		t.log("not reading query history: %s", err)
		return
	}
	defer fd.Close()
	if _, err := t.l.ReadHistory(fd); err != nil {
		t.log("error reading query history: %s", err)
	}
}

// writeHistory saves history via a temp file, so an interrupted write can not
// truncate what was there before. Queries carry hostnames and fact values, so
// the file is kept private
func (t *termReader) writeHistory() {
	if len(t.historyFile) == 0 {
		return
	}
	dir := filepath.Dir(t.historyFile)
	tmp, err := os.CreateTemp(dir, filepath.Base(t.historyFile)+".*")
	if err != nil {
		t.log("error saving query history: %s", err)
		return
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		t.log("error setting query history permissions: %s", err)
	}
	if _, err := t.l.WriteHistory(tmp); err != nil {
		tmp.Close()
		t.log("error saving query history: %s", err)
		return
	}
	if err := tmp.Close(); err != nil {
		t.log("error saving query history: %s", err)
		return
	}
	if err := os.Rename(tmp.Name(), t.historyFile); err != nil {
		t.log("error saving query history: %s", err)
	}
}

// NewLineReader picks a line reader for the given input: a full terminal one
// when it is a tty, a plain line reader when input is piped or redirected.
// The bool reports whether the session is interactive
func NewLineReader(s *Session, historyFile string, in *os.File) (LineReader, bool) {
	// liner drives both ends of the terminal, so a redirected stdout disables it
	// just as much as piped input does
	if !liner.TerminalSupported() || !isTerminal(in) || !isTerminal(os.Stdout) {
		return NewScriptReader(in), false
	}
	return NewTermReader(s, historyFile), true
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
