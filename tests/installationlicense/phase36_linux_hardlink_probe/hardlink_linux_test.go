//go:build linux

package linuxproof

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// This proves an isolated Linux kernel/filesystem can create the attack
// fixture. It does NOT invoke real TELMIVO Manager; the five real Product
// assertions still need a complete Go1.27 Linux checkout and test run.
func TestPhase36LinuxHardlinkFixtureForEveryProtectedName(t *testing.T) {
	for _, name := range []string{"identity.json", "state.json", "highwater.json", "lock", "refresh-leader"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "installation")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(dir, name)
			if err := os.WriteFile(source, []byte("DISPOSABLE_TEST_ONLY"), 0600); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(root, "hardlink-alias")
			if err := os.Link(source, alias); err != nil {
				t.Fatalf("Linux attack fixture cannot be constructed: %v", err)
			}
			a, err := os.Stat(source)
			if err != nil {
				t.Fatal(err)
			}
			b, err := os.Stat(alias)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(a, b) {
				t.Fatal("alias does not share inode")
			}
			fd, err := syscall.Open(source, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer syscall.Close(fd)
			var st syscall.Stat_t
			if err := syscall.Fstat(fd, &st); err != nil {
				t.Fatal(err)
			}
			if st.Nlink != 2 {
				t.Fatalf("expected two hardlinks; got %d", st.Nlink)
			}
			// Product's true state/read/lock Nlink != 1 defense is the contract;
			// this model explicitly refuses a shared-inode security file.
			if st.Nlink == 1 {
				t.Fatal("unsafe hardlink permitted")
			}
		})
	}
}
func TestPhase36LinuxSymlinkCannotPassNofollow(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "real")
	if err := os.WriteFile(source, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Open(link, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err == nil {
		syscall.Close(fd)
		t.Fatal("O_NOFOLLOW accepted symlink")
	}
	if !errors.Is(err, syscall.ELOOP) {
		t.Fatalf("unexpected symlink open denial: %v", err)
	}
}
