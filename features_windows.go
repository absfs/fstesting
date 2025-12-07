//go:build windows

package fstesting

// OSFeatures returns features appropriate for testing with the OS filesystem
// on Windows. Symlinks require elevated privileges and Unix permissions don't
// apply, so these are disabled by default.
func OSFeatures() Features {
	return Features{
		Symlinks:      false, // Requires admin/developer mode
		HardLinks:     false, // NTFS supports but behavior differs
		Permissions:   false, // Unix permissions don't apply to Windows
		Timestamps:    true,
		CaseSensitive: false, // Windows is case-insensitive by default
		AtomicRename:  true,
		SparseFiles:   true,
		LargeFiles:    true,
	}
}
