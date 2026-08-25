package game

import "errors"

var (
	ErrNonPlayableGame = errors.New("non playable game")
	ErrBrokenGame      = errors.New("broken game")
)
