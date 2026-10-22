package cli

// Códigos de salida según §8.
const (
	ExitOK          = 0
	ExitBug         = 1
	ExitUsage       = 2
	ExitConfig      = 3
	ExitAPI         = 4
	ExitNetwork     = 5
	ExitInterrupted = 130 // Ctrl-C del usuario (convención POSIX)
)
