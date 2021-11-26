package config

var (
	debug bool = false
)

func Debug() bool {
	return debug
}

func SetDebug(newDebug bool) {
	debug = newDebug
}
