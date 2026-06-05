// Package domain holds briefkasten's bounded context: the ports and
// invariants of a mailbox served to agents and humans. It imports no
// infrastructure — backends, transports, and presentation all depend on
// this package, never the reverse.
package domain

import "errors"

// Mailbox is the core port: anything that can list unread messages,
// fetch raw RFC 5322 bytes, and mark a message as seen.
type Mailbox interface {
	// ListUnread returns the ids of messages not yet marked seen.
	ListUnread() ([]string, error)
	// Fetch returns the raw message bytes for an unread id.
	Fetch(id string) ([]byte, error)
	// MarkSeen marks a message as processed so it is not listed again.
	MarkSeen(id string) error
}

// Searcher is an optional Mailbox capability: full-text search over the
// unread backlog.
type Searcher interface {
	// Search returns the unread ids whose raw content matches the query
	// (case-insensitive).
	Search(query string) ([]string, error)
}

// FolderMailbox is an optional Mailbox capability: backends with multiple
// folders list them and hand out folder-scoped instances.
type FolderMailbox interface {
	// Folders returns the available folder names; the default folder is
	// included (as "INBOX" for the dir backend).
	Folders() ([]string, error)
	// InFolder returns a Mailbox scoped to the named folder.
	InFolder(name string) (Mailbox, error)
}

// Curator is an optional Mailbox capability: human curation of the
// unread backlog. Both operations are soft moves — Archive files the
// message away, Delete moves it to trash. Nothing is ever expunged;
// data is never destroyed.
type Curator interface {
	// Archive moves an unread message to the archive.
	Archive(id string) error
	// Delete moves an unread message to the trash.
	Delete(id string) error
}

// ErrBadID rejects message ids that try to escape the mailbox.
var ErrBadID = errors.New("briefkasten: invalid message id")
