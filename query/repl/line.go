package repl

import (
	"bufio"
	"errors"
	"io"
)

// ErrInterrupted is returned by a LineReader when the user pressed Ctrl-C
var ErrInterrupted = errors.New("interrupted")

// LineReader provides input lines to the session. io.EOF ends the session
type LineReader interface {
	Prompt(prompt string) (string, error)
	Close() error
}

// scriptReader reads lines from a reader, for pipes, -e expressions and tests
type scriptReader struct {
	sc *bufio.Scanner
}

// NewScriptReader reads input without any terminal handling
func NewScriptReader(r io.Reader) LineReader {
	return &scriptReader{sc: bufio.NewScanner(r)}
}

func (s *scriptReader) Prompt(prompt string) (string, error) {
	if !s.sc.Scan() {
		if err := s.sc.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return s.sc.Text(), nil
}

func (s *scriptReader) Close() error { return nil }
