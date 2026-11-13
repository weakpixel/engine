//go:build race

package tilemap

// raceEnabled is true under -race, where instrumentation allocates.
const raceEnabled = true
