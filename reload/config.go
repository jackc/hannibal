package reload

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

type AppConfig struct {
	Routes []Route
}

type Route struct {
	Path   string
	Func   string
	Params []RouteParams
}

type RouteParams struct {
	Name string
	Type string
}

func (ac *AppConfig) Merge(other *AppConfig) {
	ac.Routes = append(ac.Routes, other.Routes...)
}

func New(yml []byte) (*AppConfig, error) {
	ac := &AppConfig{}
	err := yaml.UnmarshalStrict(yml, &ac)
	if err != nil {
		return nil, err
	}
	return ac, nil
}

func Load(path string) (*AppConfig, error) {
	filesFound := 0
	appConfig := &AppConfig{}

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
			ac, err := New(yml)
			if err != nil {
				return err
			}

			appConfig.Merge(ac)

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

	return appConfig, nil
}
