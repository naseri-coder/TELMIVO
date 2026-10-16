package installationlicense

// Phase43: TEST-ONLY conservative global uncertainty fence. It does not turn
// a synthetic provider receipt into real-world settlement or a production
// durable outbox. The lock+directory scan and writeExclusive share one flock:
// distinct request IDs cannot cross an unknown result from another worker.
import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"syscall"
)

var phase43Unresolved = errors.New("PHASE43_TEST_GLOBAL_RECONCILE_REQUIRED")

func phase43Hex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// phase43ScanLocked uses the anchored, already-open journal dirfd, and must
// ONLY be called while holding phase37Journal.locked's verified flock.
func phase43ScanLocked(j *phase37Journal) (bool, error) {
	fd, err := syscall.Openat(int(j.directory.Fd()), ".", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return false, phase37Corrupt
	}
	f := os.NewFile(uintptr(fd), "phase43-anchored-dir")
	entries, readErr := f.Readdirnames(-1)
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || len(entries) > 4096 {
		return false, phase37Corrupt
	}
	reservations := map[string]phase37Record{}
	confirms := map[string]bool{}
	for _, name := range entries {
		if name == "_owner.lock" {
			continue
		}
		if len(name) != 66 || !phase43Hex64(name[2:]) {
			return false, phase37Corrupt
		}
		key := name[2:]
		switch name[:2] {
		case "r-":
			var r phase37Record
			if err = j.read(name, &r); err != nil || r.Version != phase37Version ||
				r.RequestID == "" || len(r.RequestID) > 96 || phase37Name(r.RequestID) != key ||
				!phase43Hex64(r.ClaimDigest) {
				return false, phase37Corrupt
			}
			reservations[key] = r
		case "c-":
			confirms[key] = true
		default:
			return false, phase37Corrupt
		}
	}
	for k := range confirms {
		if _, ok := reservations[k]; !ok {
			return false, phase37Corrupt
		}
	}
	pending := false
	for key, r := range reservations {
		if !confirms[key] {
			pending = true
			continue
		}
		var receipt phase37Receipt
		if err = j.read("c-"+key, &receipt); err != nil {
			return false, phase37Corrupt
		}
		body, _ := json.Marshal(receipt.body())
		sig, e := hex.DecodeString(receipt.Signature)
		if e != nil || len(sig) != ed25519.SignatureSize || receipt.Version != phase37Version ||
			receipt.RequestID != r.RequestID || receipt.ClaimDigest != r.ClaimDigest ||
			receipt.ProviderID == "" || len(receipt.ProviderID) > 96 || receipt.Result != "CONFIRMED" ||
			!ed25519.Verify(j.provider, append([]byte(phase37ReceiptDomain), body...), sig) ||
			!bytes.Equal(phase37Encode(receipt), phase37Encode(&receipt)) {
			return false, phase37Corrupt
		}
	}
	return pending, nil
}

func (j *phase37Journal) phase43Pending() (bool, error) {
	pending := false
	err := j.locked(func() error { var e error; pending, e = phase43ScanLocked(j); return e })
	return pending, err
}

func (j *phase37Journal) phase43ReserveWhenSettled(c phase36Claim) error {
	if c.Protocol != phase36Protocol || c.RequestID == "" || len(c.RequestID) > 96 {
		return phase37Corrupt
	}
	return j.locked(func() error {
		name := "r-" + phase37Name(c.RequestID)
		var prior phase37Record
		e := j.read(name, &prior)
		if e == nil {
			return phase37Duplicate
		}
		if !errors.Is(e, syscall.ENOENT) {
			return phase37Corrupt
		}
		pending, err := phase43ScanLocked(j)
		if err != nil {
			return err
		}
		if pending {
			return phase43Unresolved
		}
		return j.writeExclusive(name, phase37Record{Version: phase37Version, RequestID: c.RequestID, ClaimDigest: phase37Digest(c)})
	})
}
