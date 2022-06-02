package clamp

// Clamp denotes is a flag is constant or will vary
type Clamp int

const (
	AlwaysOff Clamp = iota
	AlwaysOn
	MayVary
)
