package fstesting_test

import (
	"testing"

	"github.com/absfs/fstesting"
	"github.com/absfs/osfs"
)

// TestSuiteWithOS verifies the Suite works correctly against the real OS filesystem.
// This serves as both a test of fstesting and a baseline for expected behavior.
func TestSuiteWithOS(t *testing.T) {
	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("failed to create osfs: %v", err)
	}

	suite := &fstesting.Suite{
		FS:       osFS,
		Features: fstesting.DefaultFeatures(),
	}

	suite.Run(t)
}

func TestQuickCheck(t *testing.T) {
	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("failed to create osfs: %v", err)
	}

	suite := &fstesting.Suite{
		FS:       osFS,
		Features: fstesting.DefaultFeatures(),
	}

	suite.QuickCheck(t)
}
