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
	CSRFProtection *CSRFProtection `yaml:"csrf-protection"`
	Routes         []Route
	Services       []*Service
}

type CSRFProtection struct {
	Disable        bool
	CookieName     *string `yaml:"cookie-name"`
	Domain         *string
	FieldName      *string `yaml:"field-name"`
	HTTPOnly       *bool   `yaml:"http-only"`
	MaxAge         *int    `yaml:"max-age"`
	Path           *string
	RequestHeader  *string `yaml:"request-header"`
	SameSite       *string `yaml:"same-site"`
	Secure         *bool
	TrustedOrigins []string `yaml:"trusted-origins"`
	ErrorFunc      *string  `yaml:"error-func"`
}

type Route struct {
	GetPath               string `yaml:"get"`
	PostPath              string `yaml:"post"`
	PutPath               string `yaml:"put"`
	PatchPath             string `yaml:"patch"`
	DeletePath            string `yaml:"delete"`
	Path                  string
	Func                  string
	ReverseProxy          string               `yaml:"reverse-proxy"`
	DisableCSRFProtection bool                 `yaml:"disable-csrf-protection"`
	Params                []*RequestParam      `yaml:"params"`
	DigestPassword        *DigestPassword      `yaml:"digest-password"`
	CheckPasswordDigest   *CheckPasswordDigest `yaml:"check-password-digest"`
}

type RequestParam struct {
	Name         string
	Type         string
	ArrayElement *RequestParam   `yaml:"array-element"`
	ObjectFields []*RequestParam `yaml:"object-fields"`
	TrimSpace    *bool           `yaml:"trim-space"`
	Required     bool
	NullifyEmpty bool `yaml:"nullify-empty"`
}

type DigestPassword struct {
	PasswordParam string `yaml:"password-param"`
	DigestParam   string `yaml:"digest-param"`
}

type CheckPasswordDigest struct {
	PasswordParam         string `yaml:"password-param"`
	ResultParam           string `yaml:"result-param"`
	GetPasswordDigestFunc string `yaml:"get-password-digest-func"`
}

type Service struct {
	Name        string
	Cmd         string
	Args        []string     `yaml:",flow"`
	HTTPAddress string       `yaml:"http-address"`
	HealthCheck *HealthCheck `yaml:"health-check"`
	Blue        map[string]interface{}
	Green       map[string]interface{}
}

type HealthCheck struct {
	TCPConnect string `yaml:"tcp-connect"`
}

func (c *Config) Merge(other *Config) {
	if other.CSRFProtection != nil {
		c.CSRFProtection = other.CSRFProtection
	}
	c.Routes = append(c.Routes, other.Routes...)
	c.Services = append(c.Services, other.Services...)
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
