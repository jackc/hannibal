package appconf

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Routes []Route
}

type Route struct {
	GetPath    string `yaml:"get"`
	PostPath   string `yaml:"post"`
	PutPath    string `yaml:"put"`
	PatchPath  string `yaml:"patch"`
	DeletePath string `yaml:"delete"`
	Path       string
	Func       string
	Params     []*RequestParam `yaml:"params"`
}

type RequestParam struct {
	Name         string
	Type         string
	TrimSpace    *bool `yaml:"trim-space"`
	Required     bool
	NullifyEmpty bool `yaml:"nullify-empty"`
}

func (c *Config) Merge(other *Config) {
	c.Routes = append(c.Routes, other.Routes...)
}

func New(yml []byte) (*Config, error) {
	c := &Config{}
	err := yaml.UnmarshalStrict(yml, &c)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func Load(path string) (*Config, error) {
	filesFound := 0
	config := &Config{}

	walkFunc := func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("failed to walk for %s: %v", path, walkErr)
		}

		if info.Mode().IsRegular() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
			filesFound += 1
			yml, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			c, err := New(yml)
			if err != nil {
				return err
			}

			config.Merge(c)

			return nil
		}

		return nil
	}

	err := filepath.Walk(path, walkFunc)
	if err != nil {
		return nil, err
	}

	if filesFound == 0 {
		return nil, fmt.Errorf("no yml files found in %s", path)
	}

	return config, nil
}
