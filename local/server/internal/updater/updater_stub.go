//go:build !cli

package updater

import "errors"

var errUnsupported = errors.New("updates are not supported in this build")

type stubUpdater struct{}

func (stubUpdater) Check() (*UpdateResult, error) {
	return nil, errUnsupported
}

func (stubUpdater) Apply(*UpdateResult) error {
	return errUnsupported
}

func getUpdater(_ string) Updater {
	return stubUpdater{}
}
