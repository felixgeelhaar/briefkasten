package maildir

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.klarlabs.de/briefkasten/domain"
)

func TestDirMailboxSearch(t *testing.T) {
	mb, root := newDir(t)
	drop(t, root, "a.eml", "From: a@b.c\r\nSubject: Invoice for May\r\n\r\nhi")
	drop(t, root, "b.eml", "From: a@b.c\r\nSubject: Lunch\r\n\r\nyour INVOICE is attached")
	drop(t, root, "c.eml", "From: a@b.c\r\nSubject: Standup\r\n\r\nnotes")

	hits, err := mb.Search("invoice")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("Search hits = %v, want 2 matches", hits)
	}
	if hits[0] != "a.eml" || hits[1] != "b.eml" {
		t.Errorf("Search hits = %v, want [a.eml b.eml]", hits)
	}
}

func TestDirMailboxSearchNoMatch(t *testing.T) {
	mb, root := newDir(t)
	drop(t, root, "a.eml", "From: a@b.c\r\nSubject: A\r\n\r\nhi")

	hits, err := mb.Search("nothing-here")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("Search hits = %v, want none", hits)
	}
}

func TestDirMailboxFoldersFresh(t *testing.T) {
	mb, _ := newDir(t)

	folders, err := mb.Folders()
	if err != nil {
		t.Fatalf("Folders: %v", err)
	}
	if len(folders) != 1 || folders[0] != "INBOX" {
		t.Errorf("Folders = %v, want [INBOX]", folders)
	}
}

func TestDirMailboxFoldersListsSubMaildirs(t *testing.T) {
	mb, root := newDir(t)

	sub, err := mb.InFolder("work")
	if err != nil {
		t.Fatalf("InFolder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "work", "new", "w.eml"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	ids, err := sub.ListUnread()
	if err != nil {
		t.Fatalf("ListUnread in folder: %v", err)
	}
	if len(ids) != 1 || ids[0] != "w.eml" {
		t.Errorf("folder unread = %v, want [w.eml]", ids)
	}

	// Archiving creates the hidden .archive maildir, which must stay hidden.
	drop(t, root, "a.eml", "hi")
	if err := mb.Archive("a.eml"); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	folders, err := mb.Folders()
	if err != nil {
		t.Fatalf("Folders: %v", err)
	}
	if len(folders) != 2 || folders[0] != "INBOX" || folders[1] != "work" {
		t.Errorf("Folders = %v, want [INBOX work]", folders)
	}
}

func TestDirMailboxInFolderInbox(t *testing.T) {
	mb, _ := newDir(t)

	got, err := mb.InFolder("INBOX")
	if err != nil {
		t.Fatalf("InFolder(INBOX): %v", err)
	}
	if got != domain.Mailbox(mb) {
		t.Error("InFolder(INBOX) did not return the root mailbox")
	}
}

func TestDirMailboxInFolderCreatesMaildir(t *testing.T) {
	mb, root := newDir(t)

	if _, err := mb.InFolder("work"); err != nil {
		t.Fatalf("InFolder: %v", err)
	}
	for _, sub := range []string{"new", "cur"} {
		st, err := os.Stat(filepath.Join(root, "work", sub))
		if err != nil || !st.IsDir() {
			t.Errorf("work/%s not created: %v", sub, err)
		}
	}
}

func TestDirMailboxInFolderRejectsBadNames(t *testing.T) {
	mb, _ := newDir(t)
	for _, name := range []string{"", "../escape", "a/b", ".hidden"} {
		if _, err := mb.InFolder(name); err == nil {
			t.Errorf("InFolder(%q) accepted, want error", name)
		}
	}
}

func TestDirMailboxArchive(t *testing.T) {
	mb, root := newDir(t)
	drop(t, root, "a.eml", "hi")

	if err := mb.Archive("a.eml"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".archive", "new", "a.eml")); err != nil {
		t.Errorf("archived message missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "new", "a.eml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("message still in new/: %v", err)
	}
	ids, err := mb.ListUnread()
	if err != nil {
		t.Fatalf("ListUnread: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("unread after archive = %v, want none", ids)
	}
}

func TestDirMailboxDelete(t *testing.T) {
	mb, root := newDir(t)
	drop(t, root, "a.eml", "hi")

	if err := mb.Delete("a.eml"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".trash", "new", "a.eml")); err != nil {
		t.Errorf("trashed message missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "new", "a.eml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("message still in new/: %v", err)
	}
}

func TestDirMailboxArchiveUnknown(t *testing.T) {
	mb, _ := newDir(t)
	if err := mb.Archive("nope.eml"); err == nil {
		t.Error("Archive of unknown id accepted")
	}
}

func TestDirMailboxArchiveRejectsTraversal(t *testing.T) {
	mb, _ := newDir(t)
	err := mb.Archive("../x")
	if err == nil {
		t.Fatal("path traversal accepted in Archive")
	}
	if !errors.Is(err, domain.ErrBadID) {
		t.Errorf("Archive error = %v, want ErrBadID", err)
	}
}
