//go:build !linux

package customupgrade

import "errors"

func newStagingRoot(string) (stagingRoot, error) {
	return nil, errors.New("customupgrade: staging requires Linux")
}
