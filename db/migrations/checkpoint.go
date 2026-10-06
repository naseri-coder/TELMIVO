package migrations

import (
	"fmt"
	"strings"
)

const CurrentCheckpoint = "R5B_REMEDIATED"

// Checkpoint keeps historical release boundaries distinct from the current tree.
func Checkpoint(name string) (count int, final string, err error) {
	switch name {
	case "R4G":
		return 23, "0023_ai_assistant_2.sql", nil
	case "R5B_REMEDIATED":
		return 26, "0026_ai_ingestion_receipts.sql", nil
	case "R5B":
		return 25, "0025_apple_consumer_assistance.sql", nil
	default:
		return 0, "", fmt.Errorf("unknown migration checkpoint %q", name)
	}
}

// ValidateChain accepts only the exact ordered chain for the selected checkpoint.
func ValidateChain(names []string, checkpoint string) error {
	count, final, err := Checkpoint(checkpoint)
	if err != nil {
		return err
	}
	if len(names) != count {
		return fmt.Errorf("%s migration count=%d want=%d", checkpoint, len(names), count)
	}
	for i, name := range names {
		if !strings.HasPrefix(name, fmt.Sprintf("%04d_", i+1)) || !strings.HasSuffix(name, ".sql") {
			return fmt.Errorf("migration gap/duplicate/order at %q", name)
		}
	}
	if names[count-1] != final {
		return fmt.Errorf("unexpected final migration %q, want %q", names[count-1], final)
	}
	return nil
}

func Names() ([]string, error) {
	entries, err := Files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range entries {
		if strings.HasSuffix(f.Name(), ".sql") {
			names = append(names, f.Name())
		}
	}
	return names, nil
}
