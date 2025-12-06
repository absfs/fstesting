package fstesting

// NodeType represents the role a package plays in the absfs composition hierarchy.
type NodeType int

const (
	NodeTypeUnknown NodeType = iota
	NodeTypeCore
	NodeTypeImplementation
	NodeTypeAdapter
	NodeTypeWrapper
	NodeTypeCompositor
	NodeTypeConsumer
)

func (n NodeType) String() string {
	return [...]string{
		"Unknown",
		"Core",
		"Implementation",
		"Adapter",
		"Wrapper",
		"Compositor",
		"Consumer",
	}[n]
}

// Features describes optional capabilities of a filesystem implementation.
// Used to skip tests for features an implementation doesn't support.
type Features struct {
	// Symlinks indicates the filesystem supports symbolic links
	Symlinks bool

	// HardLinks indicates the filesystem supports hard links
	HardLinks bool

	// Permissions indicates the filesystem supports Unix-style permissions
	Permissions bool

	// Timestamps indicates the filesystem supports atime/mtime
	Timestamps bool

	// CaseSensitive indicates paths are case-sensitive
	CaseSensitive bool

	// AtomicRename indicates rename operations are atomic
	AtomicRename bool

	// SparseFiles indicates the filesystem supports sparse files
	SparseFiles bool

	// LargeFiles indicates the filesystem supports files > 2GB
	LargeFiles bool
}

// DefaultFeatures returns features typical of a full POSIX filesystem.
func DefaultFeatures() Features {
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

// MinimalFeatures returns the minimum feature set all implementations should support.
func MinimalFeatures() Features {
	return Features{
		CaseSensitive: true,
	}
}
