package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	Hub struct {
		URL    string `yaml:"url"`
		RoomID string `yaml:"roomId"`
		Token  string `yaml:"token"`
	} `yaml:"hub"`
}

func DefaultConfig() AppConfig {
	var cfg AppConfig
	cfg.Hub.URL = "https://sync.dpslogs.com"
	cfg.Hub.RoomID = ""
	cfg.Hub.Token = ""
	return cfg
}

func LoadConfig() (cfg AppConfig, path string, err error) {
	cfg = DefaultConfig()

	envPath := strings.TrimSpace(os.Getenv("DPSLOGS_CONFIG"))
	if envPath != "" {
		b, readErr := os.ReadFile(envPath)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				return cfg, "", nil
			}
			return cfg, envPath, readErr
		}
		path = envPath
		var raw AppConfig
		if unmarshalErr := yaml.Unmarshal(b, &raw); unmarshalErr != nil {
			return cfg, path, unmarshalErr
		}
		if strings.TrimSpace(raw.Hub.URL) != "" {
			cfg.Hub.URL = strings.TrimSpace(raw.Hub.URL)
		}
		if strings.TrimSpace(raw.Hub.RoomID) != "" {
			cfg.Hub.RoomID = strings.TrimSpace(raw.Hub.RoomID)
		}
		if strings.TrimSpace(raw.Hub.Token) != "" {
			cfg.Hub.Token = strings.TrimSpace(raw.Hub.Token)
		}
		return cfg, path, nil
	}

	candidates := candidateConfigPaths()
	for _, p := range candidates {
		if strings.TrimSpace(p) == "" {
			continue
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			// Found path, but cannot read.
			return cfg, p, readErr
		}
		path = p

		var raw AppConfig
		if unmarshalErr := yaml.Unmarshal(b, &raw); unmarshalErr != nil {
			return cfg, path, unmarshalErr
		}

		// Overlay non-empty values.
		if strings.TrimSpace(raw.Hub.URL) != "" {
			cfg.Hub.URL = strings.TrimSpace(raw.Hub.URL)
		}
		if strings.TrimSpace(raw.Hub.RoomID) != "" {
			cfg.Hub.RoomID = strings.TrimSpace(raw.Hub.RoomID)
		}
		if strings.TrimSpace(raw.Hub.Token) != "" {
			cfg.Hub.Token = strings.TrimSpace(raw.Hub.Token)
		}

		return cfg, path, nil
	}

	return cfg, "", nil
}

func candidateConfigPaths() []string {
	var out []string

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		out = append(out, filepath.Join(exeDir, "dpslogs.yaml"))
	}

	if base, err := os.UserConfigDir(); err == nil {
		folder := "dpslogs"
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			folder = "DPSLogs"
		}
		out = append(out, filepath.Join(base, folder, "dpslogs.yaml"))
	}

	return out
}
