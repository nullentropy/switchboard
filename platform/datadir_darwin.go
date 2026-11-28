package platform

import "os"

func DataDir() (string, error) {
	return os.UserConfigDir()
}
