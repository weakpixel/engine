package speech

import (
	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/dialogue"

	"github.com/weakpixel/ecs"
)

// EventKind is what happened to a line of [Speech.Say] or in a dialogue
// box.
type EventKind uint8

// Event kinds.
const (
	// Started: Say began a line (once its voice was loaded).
	Started EventKind = iota
	// Completed: the line played to its end, all text shown.
	Completed
	// Skipped: Skip ended the line early, all text shown.
	Skipped
	// Stopped: Stop, a new Say, Close, the entity's deletion or the loss
	// of its Speaker ended the line where it was.
	Stopped
	// LineStarted: a dialogue box's line passed its delay and began to
	// speak. Line is the line's index in the dialogue file.
	LineStarted
	// LineDone: a dialogue box's line completed or was skipped by Next;
	// all its text is shown. Line is its index.
	LineDone
	// DialogueFinished: a dialogue box's last line gave way, or Play found
	// no dialogue to play (zero, missing or failed). Line is the index of
	// the last line played. StopDialogue and the box's deletion send none.
	DialogueFinished
)

var eventNames = [...]string{"started", "completed", "skipped", "stopped", "line started", "line done", "dialogue finished"}

// String returns the kind in words: "started", "completed", "skipped",
// "stopped", "line started", "line done" or "dialogue finished".
func (k EventKind) String() string {
	if int(k) < len(eventNames) {
		return eventNames[k]
	}
	return "event(?)"
}

// Event is something that happened to a line. Read them with a
// system.EventReader on system.EventsOf[speech.Event]; the speech system
// sends them in StagePost, so a reader in StageSimulate sees them the tick
// after.
type Event struct {
	// Entity is the speaking entity or the dialogue box. It may no longer
	// be alive (a line that ended because the entity was deleted).
	Entity ecs.Entity
	// Kind is what happened.
	Kind EventKind
	// Line numbers the entity's lines from 1, one per Say, so a Stopped
	// event of the line a new Say replaced can be told from the new one's
	// Started.
	// For the dialogue kinds it is the index of the line in the dialogue
	// file, from 0.
	Line uint64
}

// eventKinds maps the library's kinds to ours (the same order today; the
// table keeps them independent).
var eventKinds = [...]EventKind{
	voice.Started:   Started,
	voice.Completed: Completed,
	voice.Skipped:   Skipped,
	voice.Stopped:   Stopped,
}

// dialogueKinds maps the dialogue package's kinds to ours.
var dialogueKinds = [...]EventKind{
	dialogue.LineStarted:      LineStarted,
	dialogue.LineDone:         LineDone,
	dialogue.DialogueFinished: DialogueFinished,
}
