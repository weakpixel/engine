//go:build race

package render

// raceEnabled is true under -race, where instrumentation allocates.
const raceEnabled = true
