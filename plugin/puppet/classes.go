package puppet

import (
	"bufio"
	"os"
	"sync"
)

type Classes struct {
	classes *map[string]interface{}
	path    string
	l       sync.Mutex
}

// load list of puppet classess
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
		return err
	}
	defer fd.Close()
	classes := make(map[string]interface{}, 0)

	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		classes[scanner.Text()] = true
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	f.l.Lock()
	defer f.l.Unlock()
	// in case we get empty YAML do not update
	if len(classes) > 0 {
		f.classes = &classes
	}
	return nil
}

// MapGetter interface
func (f *Classes) Map() *map[string]interface{} {
	f.l.Lock()
	defer f.l.Unlock()
	return f.classes
}
