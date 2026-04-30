package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
)

// BackgroundOpts are the options passed from the serve command.
type BackgroundOpts struct {
	ConfigPath string // --config flag value, may be empty
	Port       int    // --port flag value, 0 means default
}

// ForkIntoBackground starts the current binary as a detached background process.
func ForkIntoBackground(opts BackgroundOpts) error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}
	if err := paths.EnsureConfigDir(); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}
	if pid, err := GetPID(paths.PIDFile); err == nil {
		if IsProcessRunning(pid) {
			return fmt.Errorf("server is already running (PID %d)", pid)
		}
		_ = os.Remove(paths.PIDFile)
	}

	// Build args for the child process: oc-go-cc serve --_daemonize [--config X] [--port N]
	args := []string{"serve", "--_daemonize"}
	if opts.ConfigPath != "" {
		configPath, err := filepath.Abs(opts.ConfigPath)
		if err != nil {
			return fmt.Errorf("cannot resolve config path: %w", err)
		}
		args = append(args, "--config", configPath)
	}
	if opts.Port != 0 {
		args = append(args, "--port", strconv.Itoa(opts.Port))
	}

	// Open log file for the background process
	logFile, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	cmd := newBackgroundCommand(paths.BinaryPath, args)
	cmd.Env = os.Environ()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Dir = paths.ConfigDir // Run from a stable directory to avoid caller cwd issues

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start background process: %w", err)
	}

	fmt.Printf("Started %s in background\n", AppName)
	fmt.Printf("  Launcher PID: %d\n", cmd.Process.Pid)
	fmt.Printf("  Log file: %s\n", paths.LogFile)
	fmt.Printf("  PID file: %s\n", paths.PIDFile)
	fmt.Printf("  Stop with: %s stop\n", AppName)

	return nil
}

// DaemonizeSetup is called by the child process (when --_daemonize is set).
// It redirects stdout/stderr to the log file and writes the PID file.
func DaemonizeSetup(paths *Paths) error {
	// Redirect stdout and stderr to log file
	logFile, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file: %w", err)
	}

	// Replace file descriptors so slog (which writes to os.Stdout) works
	os.Stdout = logFile
	os.Stderr = logFile

	// Re-initialize the default logger to use the new stdout
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Write PID file
	if err := WritePID(paths.PIDFile, os.Getpid()); err != nil {
		return fmt.Errorf("cannot write PID file: %w", err)
	}

	return nil
}
