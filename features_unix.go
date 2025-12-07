//go:build !windows

package fstesting

// OSFeatures returns features appropriate for testing with the OS filesystem
// on Unix-like systems (Linux, macOS, BSD).
func OSFeatures() Features {
	return Features{
		Symlinks:      true,
		HardLinks:     true,
		Permissions:   true,
		Timestamps:    true,
		CaseSensitive: true,
		AtomicRename:  true,
		SparseFiles:   true,
		LargeFiles:    true,
	}
}
