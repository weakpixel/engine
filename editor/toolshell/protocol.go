package toolshell

import (
	"encoding/json"
	"slices"
	"sync"
)

// The tool protocol is JSON objects with a "type", over the same page
// transport as the level editor (hostlink.Version is sent in ready). See
// doc/design/tool-shell.md#tools for the table.

// Transport sends one message to the host. It is called on the game
// goroutine. It is the same shape as hostlink.Transport,
// which hostlink.ConnectPage returns.
type Transport interface {
	Send(msg []byte)
}

// Inbox queues the messages from the host until the next tick. It is safe
// for concurrent use: a JavaScript callback pushes, the shell drains on the
// game goroutine.
type Inbox struct {
	mu   sync.Mutex
	msgs [][]byte
}

// Push queues one message. It copies msg.
func (b *Inbox) Push(msg []byte) {
	b.mu.Lock()
	b.msgs = append(b.msgs, slices.Clone(msg))
	b.mu.Unlock()
}

// drain returns the queued messages, oldest first, and empties the queue.
func (b *Inbox) drain() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	msgs := b.msgs
	b.msgs = nil
	return msgs
}

// Tool to host.
type (
	// readyMsg is sent on the first tick. Document says the tool wants the
	// file's text (the host answers with a document message).
	readyMsg struct {
		Type     string `json:"type"`
		Version  int    `json:"version"`
		Tool     string `json:"tool"`
		Path     string `json:"path"`
		Document bool   `json:"document"`
		Panel    Panel  `json:"panel"`
	}
	// panelMsg is sent when the panel changed.
	panelMsg struct {
		Type  string `json:"type"`
		Panel Panel  `json:"panel"`
	}
	// editMsg replaces the document with Text.
	editMsg struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	// createMsg asks the host to write a new document and the files it
	// names, and to open the new document in the tool. The paths are asset
	// keys; the file data is base64 in the JSON.
	createMsg struct {
		Type  string          `json:"type"`
		Path  string          `json:"path"`
		Text  string          `json:"text"`
		Files []createFileMsg `json:"files,omitempty"`
	}
	createFileMsg struct {
		Path string `json:"path"`
		Data []byte `json:"data"`
	}
	// openMsg asks the host to open the file at asset key Path (a request:
	// the host decides how). It is the reverse of the host's open message.
	openMsg struct {
		Type string `json:"type"`
		Path string `json:"path"`
	}
	// errorMsg reports a failure.
	errorMsg struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
)

// inbound is any host-to-tool message; the type says which fields are set.
type inbound struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Value json.RawMessage `json:"value"`
	Paths []string        `json:"paths"`
	Path  string          `json:"path"`
}

// send marshals msg and sends it. A nil transport drops it.
func send(t Transport, msg any) {
	if t == nil {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		panic("toolshell: " + err.Error())
	}
	t.Send(data)
}
