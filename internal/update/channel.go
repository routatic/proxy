package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Channel represents an update channel
type Channel string

const (
	ChannelStable Channel = "stable"
	ChannelBeta   Channel = "beta"
)

// ChannelConfig stores the user's preferred update channel
type ChannelConfig struct {
	Channel Channel `json:"channel"`
}

// getChannelConfigPath returns the path to the channel config file
func getChannelConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config directory: %w", err)
	}

	appConfigDir := filepath.Join(configDir, "routatic-proxy")
	if err := os.MkdirAll(appConfigDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	return filepath.Join(appConfigDir, "update-channel.json"), nil
}

// GetChannel returns the user's preferred update channel, defaulting to stable
func GetChannel() (Channel, error) {
	configPath, err := getChannelConfigPath()
	if err != nil {
		return ChannelStable, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file yet, return default
			return ChannelStable, nil
		}
		return ChannelStable, fmt.Errorf("failed to read channel config: %w", err)
	}

	var config ChannelConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ChannelStable, fmt.Errorf("failed to parse channel config: %w", err)
	}

	// Validate channel
	if config.Channel != ChannelStable && config.Channel != ChannelBeta {
		return ChannelStable, nil
	}

	return config.Channel, nil
}

// SetChannel saves the user's preferred update channel
func SetChannel(channel Channel) error {
	if channel != ChannelStable && channel != ChannelBeta {
		return fmt.Errorf("invalid channel: %s (must be 'stable' or 'beta')", channel)
	}

	configPath, err := getChannelConfigPath()
	if err != nil {
		return err
	}

	config := ChannelConfig{Channel: channel}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal channel config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write channel config: %w", err)
	}

	return nil
}
