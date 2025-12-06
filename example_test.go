package fstesting_test

import (
	"fmt"
	"testing"

	"github.com/absfs/fstesting"
	"github.com/absfs/osfs"
)

func Example() {
	// Create a filesystem to test
	fs, err := osfs.NewFS()
	if err != nil {
		panic(err)
	}

	// Configure and run the test suite
	suite := &fstesting.Suite{
		FS:       fs,
		Features: fstesting.DefaultFeatures(),
	}

	// In a real test, you'd call suite.Run(t)
	fmt.Println("Suite configured with", countFeatures(suite.Features), "features enabled")
	// Output: Suite configured with 8 features enabled
}

func ExampleSuite_minimal() {
	fs, _ := osfs.NewFS()

	// Minimal filesystem with limited features
	suite := &fstesting.Suite{
		FS: fs,
		Features: fstesting.Features{
			CaseSensitive: true,
			AtomicRename:  true,
			// All other features disabled
		},
	}

	fmt.Println("Suite configured with", countFeatures(suite.Features), "features enabled")
	// Output: Suite configured with 2 features enabled
}

func ExampleFeatures() {
	// Default: all features enabled (OS-like filesystem)
	full := fstesting.DefaultFeatures()
	fmt.Println("Default features:", countFeatures(full))

	// Custom: in-memory filesystem without symlinks
	limited := fstesting.Features{
		Permissions:   true,
		Timestamps:    true,
		CaseSensitive: true,
		AtomicRename:  true,
	}
	fmt.Println("Limited features:", countFeatures(limited))
	// Output:
	// Default features: 8
	// Limited features: 4
}

// Helper to count enabled features
func countFeatures(f fstesting.Features) int {
	count := 0
	if f.Symlinks {
		count++
	}
	if f.HardLinks {
		count++
	}
	if f.Permissions {
		count++
	}
	if f.Timestamps {
		count++
	}
	if f.CaseSensitive {
		count++
	}
	if f.AtomicRename {
		count++
	}
	if f.SparseFiles {
		count++
	}
	if f.LargeFiles {
		count++
	}
	return count
}

// ExampleSuite_Run shows the typical test pattern.
// This is a testable example that demonstrates usage.
func ExampleSuite_Run() {
	fs, _ := osfs.NewFS()

	_ = &fstesting.Suite{
		FS:       fs,
		Features: fstesting.DefaultFeatures(),
	}

	// In your _test.go file:
	// suite.Run(t)

	fmt.Println("Ready to run tests")
	// Output: Ready to run tests
}

// ExampleSuite_QuickCheck shows the fast sanity check.
func ExampleSuite_QuickCheck() {
	fs, _ := osfs.NewFS()

	_ = &fstesting.Suite{
		FS:       fs,
		Features: fstesting.DefaultFeatures(),
	}

	// Quick sanity check during development:
	// suite.QuickCheck(t)

	fmt.Println("Ready for quick check")
	// Output: Ready for quick check
}

// Demonstrates running the full test suite
func TestExample_FullSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full suite in short mode")
	}

	fs, err := osfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	suite := &fstesting.Suite{
		FS:       fs,
		Features: fstesting.DefaultFeatures(),
	}

	suite.Run(t)
}

// Demonstrates running just a quick check
func TestExample_QuickCheck(t *testing.T) {
	fs, err := osfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	suite := &fstesting.Suite{
		FS:       fs,
		Features: fstesting.DefaultFeatures(),
	}

	suite.QuickCheck(t)
}

// Demonstrates testing with limited features
func TestExample_LimitedFeatures(t *testing.T) {
	fs, err := osfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Test as if this were an in-memory filesystem
	// without symlink or permission support
	suite := &fstesting.Suite{
		FS: fs,
		Features: fstesting.Features{
			Symlinks:      false,
			HardLinks:     false,
			Permissions:   false,
			Timestamps:    true,
			CaseSensitive: true,
			AtomicRename:  true,
			SparseFiles:   false,
			LargeFiles:    false,
		},
	}

	suite.Run(t)
}
