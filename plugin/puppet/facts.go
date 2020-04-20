package puppet

import (
	"gopkg.in/yaml.v3"
	"os"
)

type Facts map[string]interface{}

func LoadFacts(path string) (Facts, error) {
	fd, err := os.Open(path)
	if err != nil {return Facts{},err}
	var f Facts
	err = yaml.NewDecoder(fd).Decode(&f)
	if err != nil {return Facts{},err}
	return f,nil
}