package puppet

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

type Classes struct {
	classes *map[string]interface{}
	path    string
	l       sync.Mutex
}

// LoadClasses loads list of puppet classes from a classfile.
// Missing, unreadable or empty classfile is an error; returned object is still
// usable and can be retried via UpdateClasses() method
func LoadClasses(path string) (*Classes, error) {
	var f Classes
	f.path = path
	c := make(map[string]interface{}, 0)
	f.classes = &c
	return &f, f.UpdateClasses()
}

func (f *Classes) UpdateClasses() error {
	fd, err := os.Open(f.path)
	if err != nil {
		return fmt.Errorf("error opening classfile [%s]: %w", f.path, err)
	}
	defer fd.Close()
	classes := make(map[string]interface{}, 0)

	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		class := strings.TrimSpace(scanner.Text())
		// puppet writes one class per line, ignore whatever leftover whitespace there is
		if len(class) == 0 {
			continue
		}
		classes[class] = true
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading classfile [%s]: %w", f.path, err)
	}
	// in case we get empty classfile do not update
	if len(classes) == 0 {
		return fmt.Errorf("got no classes after parsing classfile [%s]", f.path)
	}
	f.l.Lock()
	defer f.l.Unlock()
	f.classes = &classes
	return nil
}

// MapGetter interface
func (f *Classes) Map() *map[string]interface{} {
	f.l.Lock()
	defer f.l.Unlock()
	return f.classes
}
