package domain

import "errors"

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrInvalidQuery    = errors.New("invalid query")
	ErrBlockedByPolicy = errors.New("blocked by policy")
	ErrAlreadyExists   = errors.New("already exists")
	ErrNotFound        = errors.New("not found")
)
