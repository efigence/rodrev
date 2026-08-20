package puppet

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

type Facts struct {
	facts *map[string]interface{}
	path  string
	l     sync.Mutex
}

// LoadFacts creates fact structure and loads facts into it.
// Missing, unparsable or empty fact file is an error; returned object is still
// usable and the load can be retried via UpdateFacts() method
func LoadFacts(path string) (*Facts, error) {
	var f Facts
	f.path = path
	facts := make(map[string]interface{}, 0)
	f.facts = &facts
	return &f, f.UpdateFacts()
}

// NewFactsFromMap wraps an already loaded fact map, for facts that did not come
// from a file (a fact dump received from another node). UpdateFacts() on it will
// fail as there is no path to reload from
func NewFactsFromMap(m map[string]interface{}) *Facts {
	var f Facts
	if m == nil {
		m = make(map[string]interface{}, 0)
	}
	f.facts = &m
	return &f
}

func (f *Facts) UpdateFacts() error {
	if len(f.path) == 0 {
		return fmt.Errorf("no fact file path, facts were loaded from memory")
	}
	fd, err := os.Open(f.path)
	if err != nil {
		return fmt.Errorf("error opening fact file [%s]: %w", f.path, err)
	}
	defer fd.Close()
	var facts map[string]interface{}
	err = yaml.NewDecoder(fd).Decode(&facts)
	if err != nil {
		return fmt.Errorf("error parsing fact file [%s]: %w", f.path, err)
	}
	// in case we get empty YAML do not update
	if len(facts) == 0 {
		return fmt.Errorf("got empty fact YAML after decoding [%s]", f.path)
	}
	f.l.Lock()
	defer f.l.Unlock()
	f.facts = &facts
	return nil
}

func (f *Facts) Map() *map[string]interface{} {
	f.l.Lock()
	defer f.l.Unlock()
	return f.facts
}
