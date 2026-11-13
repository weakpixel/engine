//go:build race

package extract_test

// raceEnabled is true under -race, where instrumentation allocates.
const raceEnabled = true
