package puppet

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"sync"
)

type Facts struct {
	facts *map[string]interface{}
	path  string
	l     sync.Mutex
}

// LoadFacts creates fact structure and loads fact into it
// on error it can be retired via UpdateFacts() method
func LoadFacts(path string) (Facts, error) {
	var f Facts
	f.path = path
	fd, err := os.Open(path)
	if err != nil {
		return f, err
	}
	var facts map[string]interface{}
	err = yaml.NewDecoder(fd).Decode(&facts)
	f.facts = &facts
	if err != nil {
		return f, err
	}
	return f, nil
}

func (f *Facts) UpdateFacts() error {
	fd, err := os.Open(f.path)
	if err != nil {
		return err
	}
	var facts map[string]interface{}
	err = yaml.NewDecoder(fd).Decode(&facts)
	defer fd.Close()
	if err != nil {
		return err
	}
	f.l.Lock()
	defer f.l.Unlock()
	// in case we get empty YAML do not update
	if len(facts) > 0 {
		f.facts = &facts
	} else {
		return fmt.Errorf("got empty fact YAML after decode")
	}
	return nil
}

func (f *Facts) Map() *map[string]interface{} {
	f.l.Lock()
	defer f.l.Unlock()
	return f.facts
}
