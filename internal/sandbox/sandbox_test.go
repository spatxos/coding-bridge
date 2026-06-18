package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBakSnapshotCanBeLoadedAndRolledBack(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "main.txt")
	if err := os.WriteFile(target, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}

	manager := NewSnapshotManager(root)
	snapshot, err := manager.CreateSnapshot("rollback-test")
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	if snapshot.Method != MethodBak {
		t.Fatalf("snapshot method = %s, want bak", snapshot.Method)
	}
	if err := manager.PrepareSnapshot(snapshot, []string{"main.txt", "created.txt"}); err != nil {
		t.Fatalf("PrepareSnapshot() error = %v", err)
	}

	if err := os.WriteFile(target, []byte("after"), 0644); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(root, "created.txt")
	if err := os.WriteFile(created, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := manager.LoadSnapshot("rollback-test")
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	if err := manager.Rollback(loaded); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "before" {
		t.Fatalf("restored content = %q, want before", data)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("created file still exists after rollback: %v", err)
	}
}
