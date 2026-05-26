package buildinfo

import "os"

var Version = "dev"
var BuildTime = "unknown"

func BinaryPath() string {
	path, err := os.Executable()
	if err != nil {
		return "unknown"
	}
	return path
}

func PID() int {
	return os.Getpid()
}
