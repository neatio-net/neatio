package neatcon

import "errors"

var (
	ErrUnauthorizedAddress = errors.New("unauthorized address")

	ErrStoppedEngine = errors.New("stopped engine")

	ErrStartedEngine = errors.New("started engine")

	ErrNoPrivValidator = errors.New("cannot start node without private validator")
)
