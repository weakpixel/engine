//go:build race

package game

// raceEnabled is true under -race, where instrumentation allocates.
const raceEnabled = true
