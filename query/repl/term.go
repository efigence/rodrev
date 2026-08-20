package repl

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
)

// historyLimit is how many lines are kept in the history file
const historyLimit = 1000

// DefaultHistoryFile returns the history file path in the user's home dir
func DefaultHistoryFile() string {
	home, err := os.UserHomeDir()
	if err != nil || len(home) == 0 {
		return ""
	}
	return filepath.Join(home, ".rv_query_history")
}

// termReader is a readline-backed LineReader with history and tab completion.
// This is the only part of the REPL that needs a terminal
type termReader struct {
	l           *readline.Instance
	historyFile string
	log         func(format string, args ...interface{})
}

// NewTermReader wires the line editor up to the session's completer and history
// file. An empty historyFile disables history persistence
func NewTermReader(s *Session, historyFile string) (LineReader, error) {
	if len(historyFile) > 0 {
		// queries carry hostnames and fact values, so make sure the file is
		// private before the editor starts appending to it
		if err := ensurePrivateFile(historyFile); err != nil {
			s.log.Debugf("not using query history: %s", err)
			historyFile = ""
		}
	}
	l, err := readline.NewEx(&readline.Config{
		Prompt:            s.Prompt(),
		HistoryFile:       historyFile,
		HistoryLimit:      historyLimit,
		HistorySearchFold: true,
		AutoComplete:      &completer{session: s},
		InterruptPrompt:   "^C",
	})
	if err != nil {
		return nil, err
	}
	return &termReader{l: l, historyFile: historyFile, log: s.log.Debugf}, nil
}

func (t *termReader) Prompt(prompt string) (string, error) {
	t.l.SetPrompt(prompt)
	line, err := t.l.Readline()
	switch err {
	case nil:
		return line, nil
	case readline.ErrInterrupt:
		// Ctrl-C throws away what was typed. With nothing to throw away it means
		// "get me out of here"
		return "", interruptResult(line)
	case io.EOF:
		return "", io.EOF
	default:
		return "", err
	}
}

// interruptResult turns a Ctrl-C into either "the line is gone, carry on" or
// "the line was already empty, so exit"
func interruptResult(line string) error {
	if len(strings.TrimSpace(line)) == 0 {
		return io.EOF
	}
	return ErrInterrupted
}

func (t *termReader) Close() error {
	err := t.l.Close()
	if len(t.historyFile) > 0 {
		// the editor rewrites the history file when trimming it, with default
		// permissions
		if err := ensurePrivateFile(t.historyFile); err != nil {
			t.log("could not keep query history private: %s", err)
		}
	}
	return err
}

// ensurePrivateFile creates the file if needed and makes sure only its owner can
// read it
func ensurePrivateFile(path string) error {
	fd, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()
	return fd.Chmod(0600)
}

// completer adapts the session completer to the line editor, which wants the
// candidates with the typed prefix already stripped off
type completer struct {
	session *Session
}

func (c *completer) Do(line []rune, pos int) ([][]rune, int) {
	return completeRunes(c.session.CompletionState(), line, pos)
}

// completeRunes is Complete in the shape the line editor expects: every
// candidate loses the prefix that is already on the line, and the number of
// runes it replaces is returned alongside
func completeRunes(st CompletionState, line []rune, pos int) ([][]rune, int) {
	head, completions, _ := Complete(st, string(line), pos)
	typed := pos - len([]rune(head))
	if typed < 0 {
		return nil, 0
	}
	out := make([][]rune, 0, len(completions))
	for _, candidate := range completions {
		runes := []rune(candidate)
		if len(runes) < typed {
			continue
		}
		out = append(out, runes[typed:])
	}
	return out, typed
}

// NewLineReader picks a line reader for the given input: a full terminal one
// when it is a tty, a plain line reader when input is piped or redirected.
// The bool reports whether the session is interactive
func NewLineReader(s *Session, historyFile string, in *os.File) (LineReader, bool) {
	if !isTerminal(in) || !isTerminal(os.Stdout) {
		return NewScriptReader(in), false
	}
	lr, err := NewTermReader(s, historyFile)
	if err != nil {
		s.log.Debugf("no line editing (%s), falling back to plain input", err)
		return NewScriptReader(in), false
	}
	return lr, true
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
