package utils

// Check panics when cond is false.
func Check(cond bool) {
	if !cond {
		panic("assertion failure")
	}
}
