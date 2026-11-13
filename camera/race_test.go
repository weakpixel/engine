//go:build race

package camera

// raceEnabled is true under -race, where instrumentation allocates.
const raceEnabled = true
