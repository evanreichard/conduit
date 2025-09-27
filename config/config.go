package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/pflag"
)

var version string = "develop"

type ConfigDef struct {
	Key         string
	Env         string
	Default     string
	Description string
}

type BaseConfig struct {
	ServerAddress string `json:"server" description:"Conduit server address" default:"http://localhost:8080"`
	APIKey        string `json:"api_key" description:"API Key for the conduit API"`
	LogLevel      string `json:"log_level" default:"info" description:"Log level"`
	LogFormat     string `json:"log_format" default:"text" description:"Log format - text or json"`
}

func (c *BaseConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("api_key is required")
	}
	if c.ServerAddress == "" {
		return errors.New("server is required")
	}
	if _, err := url.Parse(c.ServerAddress); err != nil {
		return fmt.Errorf("server is invalid: %w", err)
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		return fmt.Errorf("log format must be 'text' or 'json'")
	}
	return nil
}

type ServerConfig struct {
	BaseConfig
	BindAddress string `json:"bind" default:"0.0.0.0:8080" description:"Address the conduit server listens on"`
}

type ClientConfig struct {
	BaseConfig
	TunnelName   string `json:"name" description:"Tunnel name"`
	TunnelTarget string `json:"target" description:"Tunnel target address"`
}

func (c *ClientConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}
	if c.TunnelTarget == "" {
		return fmt.Errorf("target is required")
	}
	return nil
}

func GetServerConfig(cmdFlags *pflag.FlagSet) (*ServerConfig, error) {
	defs := GetConfigDefs[ServerConfig]()

	cfgValues := make(map[string]string)
	for _, def := range defs {
		cfgValues[def.Key] = getConfigValue(cmdFlags, def)
	}

	cfg := &ServerConfig{
		BaseConfig:  getBaseConfig(cfgValues),
		BindAddress: cfgValues["bind"],
	}

	// Initialize Logger
	initLogger(cfg.BaseConfig)

	return cfg, cfg.Validate()
}

func GetClientConfig(cmdFlags *pflag.FlagSet) (*ClientConfig, error) {
	defs := GetConfigDefs[ClientConfig]()

	cfgValues := make(map[string]string)
	for _, def := range defs {
		cfgValues[def.Key] = getConfigValue(cmdFlags, def)
	}

	cfg := &ClientConfig{
		BaseConfig:   getBaseConfig(cfgValues),
		TunnelName:   cfgValues["name"],
		TunnelTarget: cfgValues["target"],
	}

	// Initialize Logger
	initLogger(cfg.BaseConfig)

	return cfg, cfg.Validate()
}

func GetConfigDefs[T ServerConfig | ClientConfig]() []ConfigDef {
	var defs []ConfigDef
	processFields(reflect.TypeFor[T](), &defs)
	return defs
}

func GetVersion() string {
	return version
}

func getBaseConfig(cfgValues map[string]string) BaseConfig {
	return BaseConfig{
		ServerAddress: cfgValues["server"],
		APIKey:        cfgValues["api_key"],
		LogLevel:      cfgValues["log_level"],
		LogFormat:     cfgValues["log_format"],
	}
}

func getConfigValue(cmdFlags *pflag.FlagSet, def ConfigDef) string {
	// 1. Get Flags First
	if cmdFlags != nil {
		if val, err := cmdFlags.GetString(def.Key); err == nil && val != "" && val != def.Default {
			return val
		}
	}

	// 2. Environment Variables Next
	if envVal := os.Getenv(def.Env); envVal != "" {
		return envVal
	}

	// 3. Defaults Last
	return def.Default
}

func processFields(t reflect.Type, defs *[]ConfigDef) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Process Embedded (BaseConfig)
		if field.Anonymous {
			processFields(field.Type, defs)
			continue
		}

		// Extract Struct Tags
		jsonTag := field.Tag.Get("json")
		defaultTag := field.Tag.Get("default")
		descriptionTag := field.Tag.Get("description")

		// Skip JSON Fields
		if jsonTag == "" {
			continue
		}

		// Get Key & Env
		key := strings.Split(jsonTag, ",")[0]
		env := "CONDUIT_" + strings.ToUpper(key)

		*defs = append(*defs, ConfigDef{
			Key:         key,
			Env:         env,
			Default:     defaultTag,
			Description: descriptionTag,
		})
	}
}
