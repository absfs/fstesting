package fstesting

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/absfs/absfs"
	"github.com/pkg/errors"
)

type Testcase struct {
	TestNo         int         `json:"test_no"`
	PreCondition   string      `json:"pre_condition"`
	Op             string      `json:"op"`
	Path           string      `json:"path"`
	Flags          int         `json:"flags"`
	Mode           os.FileMode `json:"mode"`
	OpenErr        error       `json:"open_err"`
	OpenErrString  string      `json:"open_err_string"`
	WriteErr       error       `json:"write_err"`
	WriteErrString string      `json:"write_err_string"`
	ReadErr        error       `json:"read_err"`
	ReadErrString  string      `json:"read_err_string"`
	CloseErr       error       `json:"close_err"`
	CloseErrString string      `json:"close_err_string"`
	log            []string
}

// Creates a timestamped folder for filesystem testing, and changes directory
// to it.
// Returns the path to the new directory, a cleanup function that and and
// an error.
// The `cleanup` method changes the directory back to the original location
// and removes testdir and all of it's contents.
func OsTestDir(path string) (testdir string, cleanup func(), err error) {

	// assign noop to cleanup until there is something to clean up.
	cleanup = func() {}
	timestamp := time.Now().Format(time.RFC3339)
	testdir = filepath.Join(path, fmt.Sprintf("FsTestDir%s", timestamp))

	err = os.Mkdir(testdir, 0777)
	if err != nil {
		return testdir, cleanup, err
	}

	// capture the current working directory
	var startingDir string
	startingDir, err = os.Getwd()
	if err != nil {
		return testdir, cleanup, err
	}

	cleanup = func() {
		os.Chdir(startingDir)
		err := os.RemoveAll(testdir)
		if err != nil {
			panic(err)
		}
	}

	err = os.Chdir(testdir)
	return testdir, cleanup, err
}

// FsTestDir is similar to OsTestDir, but performs all operations on a
// absfs.FileSystem.
// FsTestDir Creates a timestamped folder for filesystem testing, and changes directory
// to it.
// Returns the path to the new directory, a cleanup function that and and
// an error.
// The `cleanup` method changes the directory back to the original location
// and removes testdir and all of it's contents.
func FsTestDir(fs absfs.FileSystem, path string) (testdir string, cleanup func(), err error) {

	// assign noop to cleanup until there is something to clean up.
	cleanup = func() {}
	timestamp := time.Now().Format(time.RFC3339)
	testdir = filepath.Join(path, fmt.Sprintf("FsTestDir%s", timestamp))

	err = fs.Mkdir(testdir, 0777)
	if err != nil {
		return testdir, cleanup, err
	}

	// capture the current working directory
	var startingDir string
	startingDir, err = fs.Getwd()
	if err != nil {
		return testdir, cleanup, err
	}

	cleanup = func() {
		fs.Chdir(startingDir)
		err := fs.RemoveAll(testdir)
		if err != nil {
			panic(err)
		}
	}

	err = fs.Chdir(testdir)
	return testdir, cleanup, err
}

// GenerateTestcases runs all tests on the `os` package to establish baseline
// results that can be used to test that `absfs` FileSystems are consistent with
// native file system support.
// If not `nil` GenerateTestcases will call `fn` with each generated testcase. If
// `fn` returns an error then testcase generation will stop and GenerateTestcases
// will return an the same error and the testcases crated so far.
// (TODO: many tests still to be added to exercise the entire FileSystem interface)
func GenerateTestcases(testdir string, fn func(*Testcase) error) (testcases []*Testcase, err error) {

	// Various OpenFile pre-conditions
	preconditions := []string{
		"notcreated",  // No file exists for filename.
		"created",     // A file with normal permissions exists for filename
		"dir",         // A directory with normal permissions exists for filename
		"permissions", // A file with no permissions exists for filename
	}
	testNo := 0
	if testdir == "" {
		return nil, errors.New("testdir undefined")
	}

	// define noop function if needed
	if fn == nil {
		fn = func(*Testcase) error {
			return nil
		}
	}
	err = ForEveryFlag(func(flag int) error {
		return ForEveryPermission(func(mode os.FileMode) error {
			for _, pathPrefix := range []string{testdir, ".", ""} {
				for _, condition := range preconditions {

					name := filepath.Join(pathPrefix, fmt.Sprintf("fstestingFile%08d", testNo))
					switch condition {
					case "notcreated":
					case "created":
						info, err := os.Stat(name)
						if !os.IsNotExist(err) {
							return fmt.Errorf("file exists unexpectedly %s %q", info.Mode(), name)
						}
						f, err := os.Create(name)
						if err != nil {
							return fmt.Errorf("unable to create %q + %q, %s", testdir, name, err)
						} else {
							_, err = f.WriteString("Hello, world!\n")
							if err != nil {
								return err
							}
							f.Close()
						}

					case "dir":
						name = filepath.Join(pathPrefix, fmt.Sprintf("fstestingDir%08d", testNo))
						err = os.Mkdir(name, 0777)
						if err != nil {
							return err
						}

					case "permissions":
						f, err := os.Create(name)
						if err != nil {
							return err
						}
						_, err = f.WriteString("Hello, world!\n")
						if err != nil {
							return err
						}
						f.Close()
						err = os.Chmod(name, 0)
						if err != nil {
							return err
						}
					}

					// OpenFile test
					f, openErr := os.OpenFile(name, flag, os.FileMode(mode))

					// Write test
					writedata := []byte("The quick brown fox, jumped over the lazy dog!")
					n, writeErr := f.Write(writedata)
					_ = n
					// TODO: check if n == len(writedata)

					// Read test
					f.Seek(0, io.SeekStart)
					readdata := make([]byte, 512)
					n, readErr := f.Read(readdata)
					readdata = readdata[:n]
					_ = readdata

					// Close test
					closeErr := f.Close()

					// Capture all available error strings
					var OpenErrString, WriteErrString, ReadErrString, CloseErrString string
					if openErr != nil {
						OpenErrString = openErr.Error()
					}
					if writeErr != nil {
						WriteErrString = writeErr.Error()
					}
					if readErr != nil {
						ReadErrString = readErr.Error()
					}
					if closeErr != nil {
						CloseErrString = closeErr.Error()
					}
					// Create a new test case with above values.
					testcase := &Testcase{
						TestNo:       testNo,
						PreCondition: condition,
						Op:           "openfile",
						Path:         name,
						Flags:        flag,
						Mode:         os.FileMode(mode),

						OpenErr:       openErr,
						OpenErrString: OpenErrString,

						WriteErr:       writeErr,
						WriteErrString: WriteErrString,

						ReadErr:       readErr,
						ReadErrString: ReadErrString,

						CloseErr:       closeErr,
						CloseErrString: CloseErrString,
					}

					err = fn(testcase)
					if err != nil {
						return err
					}

					testcases = append(testcases, testcase)
					testNo++
				}
			}
			return nil
		})
	})
	if err != nil {
		return testcases, err
	}

	return testcases, nil
}

func FsTest(fs absfs.FileSystem, path string, testcase *Testcase) (*Testcase, error) {
	name, err := pretest(fs, path, testcase)

	newtestcase, err := test(fs, testcase.TestNo, name, testcase.Flags, testcase.Mode, testcase.PreCondition)
	posttest(fs, newtestcase)
	return newtestcase, err
}

func createFile(fs absfs.FileSystem, name string) error {
	info, err := fs.Stat(name)
	if !os.IsNotExist(err) {
		return fmt.Errorf("file exists unexpectedly %s %q", info.Mode(), name)
	}
	f, err := fs.Create(name)
	if err != nil {
		return fmt.Errorf("unable to create  %q, %s", name, err)
	}
	defer f.Close()

	_, err = f.WriteString("Hello, world!\n")
	return err
}

func pretest(fs absfs.FileSystem, path string, testcase *Testcase) (string, error) {
	name := filepath.Join(path, fmt.Sprintf("fstestingFile%08d", testcase.TestNo))
	switch testcase.PreCondition {
	case "":
		fallthrough
	case "notcreated":

	case "created":
		err := createFile(fs, name)
		if err != nil {
			return "", err
		}

	case "dir":
		name = filepath.Join(path, fmt.Sprintf("fstestingDir%08d", testcase.TestNo))
		err := os.Mkdir(name, 0777)
		if err != nil {
			return name, err
		}

	case "permissions":
		err := createFile(fs, name)
		if err != nil {
			return name, err
		}
		err = fs.Chmod(name, 0)
		if err != nil {
			return name, err
		}
	}

	return name, nil
}

func posttest(fs absfs.FileSystem, testcase *Testcase) error {

	return nil
}

func test(fs absfs.FileSystem, testNo int, name string, flags int, mode os.FileMode, precondition string) (*Testcase, error) {

	// OpenFile test
	f, openErr := os.OpenFile(name, flags, os.FileMode(mode))

	// Write test
	writedata := []byte("The quick brown fox, jumped over the lazy dog!")
	n, writeErr := f.Write(writedata)
	_ = n
	// TODO: check if n == len(writedata)

	// Read test
	f.Seek(0, io.SeekStart)
	readdata := make([]byte, 512)
	n, readErr := f.Read(readdata)
	readdata = readdata[:n]
	_ = readdata

	// Close test
	closeErr := f.Close()

	// Capture all available error strings
	var OpenErrString, WriteErrString, ReadErrString, CloseErrString string
	if openErr != nil {
		OpenErrString = openErr.Error()
	}
	if writeErr != nil {
		WriteErrString = writeErr.Error()
	}
	if readErr != nil {
		ReadErrString = readErr.Error()
	}
	if closeErr != nil {
		CloseErrString = closeErr.Error()
	}

	// Create a new test case with above values.
	testcase := &Testcase{
		TestNo:       testNo,
		PreCondition: precondition,
		Op:           "openfile",
		Path:         name,
		Flags:        flags,
		Mode:         os.FileMode(mode),

		OpenErr:       openErr,
		OpenErrString: OpenErrString,

		WriteErr:       writeErr,
		WriteErrString: WriteErrString,

		ReadErr:       readErr,
		ReadErrString: ReadErrString,

		CloseErr:       closeErr,
		CloseErrString: CloseErrString,
	}

	return testcase, nil
}

func CompareErrors(err1 error, err2 error) error {

	switch v1 := err1.(type) {

	case nil:
		if err2 == nil {
			return nil
		}
		return fmt.Errorf("err1 is <nil> err2 is %s", err2)

	case *os.PathError:
		v2, ok := err2.(*os.PathError)
		if !ok {
			return errors.Errorf("errors differ in type %T != %T", err1, err2)
		}

		var list []string

		if v1.Path == v2.Path {
			list = append(list, fmt.Sprintf("paths not equal %q != %q", v1.Path, v2.Path))
		}
		if v1.Op != v2.Op {
			list = append(list, fmt.Sprintf("ops not equal %q != %q", v1.Op, v2.Op))
		}
		if v1.Err.Error() != v2.Err.Error() {
			list = append(list, fmt.Sprintf("errors not equal %q != %q", v1.Err.Error(), v2.Err.Error()))
		}
		if len(list) == 0 {
			return nil
		}
		return fmt.Errorf("os.PathErrors:  %s", strings.Join(list, "; "))
	}

	if err1.Error() != err2.Error() {
		return fmt.Errorf("unknown unequal errors %T & %T differ %q != %q", err1, err2, err1, err2)
	}

	return nil // fmt.Errorf("unknown matching errors %T, %q", err1, err2)
}
