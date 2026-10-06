package migrations

import "testing"

func TestMigrationCheckpointContracts(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateChain(names, CurrentCheckpoint); err != nil {
		t.Fatal(err)
	}
	if err = ValidateChain(names[:26], "R5B_REMEDIATED"); err != nil {
		t.Fatal(err)
	}
	if err = ValidateChain(names[:25], "R5B"); err != nil {
		t.Fatal(err)
	}
	if err = ValidateChain(names[:23], "R4G"); err != nil {
		t.Fatal(err)
	}
	if ValidateChain(names, "R4G") == nil || ValidateChain(names[:23], "R5B") == nil {
		t.Fatal("historical/current checkpoints were conflated")
	}
	cases := map[string][]string{
		"missing": append(append([]string{}, names[:10]...), names[11:]...),
		"extra":   append(append([]string{}, names...), "0027_unexpected.sql"),
	}
	duplicate := append([]string{}, names...)
	duplicate[10] = names[9]
	cases["duplicate"] = duplicate
	order := append([]string{}, names...)
	order[0], order[1] = order[1], order[0]
	cases["order"] = order
	final := append([]string{}, names...)
	final[len(final)-1] = "0027_unexpected.sql"
	cases["final"] = final
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			if ValidateChain(files, CurrentCheckpoint) == nil {
				t.Fatal("invalid chain accepted")
			}
		})
	}
	if ValidateChain(names, "unknown") == nil {
		t.Fatal("unknown checkpoint accepted")
	}
}
