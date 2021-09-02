package fs

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNotify(t *testing.T) {
	tests := []struct {
		scenario string
		function func(*testing.T, string, chan string)
	}{
		{
			scenario: "creating a file triggers a notification on the parent directory",
			function: testNotifyCreateFile,
		},

		{
			scenario: "creating a directory triggers a notification on the parent directory",
			function: testNotifyCreateDirectory,
		},

		{
			scenario: "creating a link triggers a notification on the parent directory",
			function: testNotifyCreateLink,
		},

		{
			scenario: "creating a symlink triggers a notification on the parent directory",
			function: testNotifyCreateSymlink,
		},

		{
			scenario: "removing a file triggers a notification on the parent directory",
			function: testNotifyRemoveFile,
		},

		{
			scenario: "removing a directory triggers a notification on the parent directory",
			function: testNotifyRemoveDirectory,
		},

		{
			scenario: "renaming a file triggers a notification on the parent directory",
			function: testNotifyRenameFile,
		},

		{
			scenario: "renaming a directory triggers a notification on the parent directory",
			function: testNotifyRenameDirectory,
		},

		{
			scenario: "moving a file out triggers a notification on the parent directory",
			function: testNotifyMoveFile,
		},

		{
			scenario: "moving a directory out triggers a notification on the parent directory",
			function: testNotifyMoveDirectory,
		},

		{
			scenario: "writing to a file triggers a notification on the file",
			function: testNotifyFileWrite,
		},

		{
			scenario: "truncating a file triggers a notification on the file",
			function: testNotifyFileTruncate,
		},

		{
			scenario: "renaming a symlink triggers a notification on the source and target",
			function: testNotifySymlinkRename,
		},

		{
			scenario: "renaming a symlink multiple times triggers a notification each time on the target",
			function: testNotifySymlinkMultiRename,
		},
	}

	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) {
			tmp, err := ioutil.TempDir("", "notify.*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmp)

			ch := make(chan string)
			defer Stop(ch)

			test.function(t, tmp, ch)
		})
	}
}

func testNotifyCreateFile(t *testing.T, dir string, ch chan string) {
	if err := Notify(ch, dir); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(dir, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	assert(t, ch, dir)
}

func testNotifyCreateDirectory(t *testing.T, dir string, ch chan string) {
	if err := Notify(ch, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "test"), 0755); err != nil {
		t.Fatal(err)
	}
	assert(t, ch, dir)
}

func testNotifyCreateLink(t *testing.T, dir string, ch chan string) {
	f, err := os.Create(filepath.Join(dir, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := Notify(ch, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(f.Name(), f.Name()+"~"); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, dir)
}

func testNotifyCreateSymlink(t *testing.T, dir string, ch chan string) {
	f, err := os.Create(filepath.Join(dir, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := Notify(ch, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.Name(), f.Name()+"~"); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, dir)
}

func testNotifyRemoveFile(t *testing.T, dir string, ch chan string) {
	f, err := os.Create(filepath.Join(dir, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := Notify(ch, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.Name()); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, dir)
}

func testNotifyRemoveDirectory(t *testing.T, dir string, ch chan string) {
	subdir := filepath.Join(dir, "test")

	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := Notify(ch, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(subdir); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, dir)
}

func testNotifyRenameFile(t *testing.T, dir string, ch chan string) {
	f, err := os.Create(filepath.Join(dir, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := Notify(ch, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.Name(), f.Name()+"~"); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, dir)
}

func testNotifyRenameDirectory(t *testing.T, dir string, ch chan string) {
	subdir := filepath.Join(dir, "test")

	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := Notify(ch, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(subdir, subdir+"~"); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, dir)
}

func testNotifyMoveFile(t *testing.T, dir string, ch chan string) {
	subdir := filepath.Join(dir, "sub")

	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(dir, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := Notify(ch, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, "sub", "test")); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, dir)
}

func testNotifyMoveDirectory(t *testing.T, dir string, ch chan string) {
	subdir1 := filepath.Join(dir, "sub1")
	subdir2 := filepath.Join(dir, "sub2")

	if err := os.Mkdir(subdir1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(subdir2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := Notify(ch, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(subdir2, filepath.Join(subdir1, "test")); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, dir)
}

func testNotifyFileWrite(t *testing.T, dir string, ch chan string) {
	f, err := os.Create(filepath.Join(dir, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := Notify(ch, f.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(f, "Hello World\n"); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, f.Name())
}

func testNotifyFileTruncate(t *testing.T, dir string, ch chan string) {
	f, err := os.Create(filepath.Join(dir, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if _, err := io.WriteString(f, "Hello World\n"); err != nil {
		t.Fatal(err)
	}
	if err := Notify(ch, f.Name()); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(0); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, f.Name())
}

func testNotifySymlinkRename(t *testing.T, dir string, ch chan string) {
	symlink1 := filepath.Join(dir, "symlink1")
	symlink2 := filepath.Join(dir, "symlink2")

	if err := os.Symlink("target1", symlink1); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target2", symlink2); err != nil {
		t.Fatal(err)
	}
	if err := Notify(ch, symlink2); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(symlink1, symlink2); err != nil {
		t.Fatal(err)
	}

	assert(t, ch, symlink2)
}

func testNotifySymlinkMultiRename(t *testing.T, dir string, ch chan string) {
	symlink1 := filepath.Join(dir, "symlink1")
	symlink2 := filepath.Join(dir, "symlink2")
	symlink3 := filepath.Join(dir, "symlink3")
	symlink4 := filepath.Join(dir, "symlink4")

	if err := os.Symlink("target1", symlink1); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target2", symlink2); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target3", symlink3); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target4", symlink4); err != nil {
		t.Fatal(err)
	}

	for _, symlink := range []string{symlink1, symlink2, symlink3} {
		if err := Notify(ch, symlink4); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(symlink, symlink4); err != nil {
			t.Fatal(err)
		}
		assert(t, ch, symlink4)
	}
}

func assert(t *testing.T, ch chan string, path string) {
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()

	var found string
	select {
	case found = <-ch:
	case <-timeout.C:
		t.Error("timeout waiting for", path)
		return
	}

	if path != found {
		t.Error("paths mismatch")
		t.Logf("expected: %q", path)
		t.Logf("found:    %q", found)
	}
}
