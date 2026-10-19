// Package session coordinates UI-independent authoring operations. Frontends own
// event-loop serialization, selection/request approvals and rendering. Publication
// is allowed to finish; cancelling a UI is not rollback of a published file.
package session

import (
	"errors"
	"sync/atomic"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

var ErrBusy = errors.New("authoring publication is still running")
var ErrExecuted = errors.New("authoring operation already executed")
var ErrDocumentRequired = errors.New("authoring document required")

// Documents admits one asynchronous document operation at a time. Controls are
// event-loop serialized; Job.Run alone executes on the worker. Zero value is ready.
type Documents struct {
	generation uint64
	active     *Job
}

type Job struct {
	owner                *Documents
	origin, working      *authoring.LibraryEditor
	generation, revision uint64
	ran                  atomic.Bool
}

type Result struct {
	job *Job
	err error
}

func (result Result) Generation() uint64 {
	if result.job == nil {
		return 0
	}
	return result.job.generation
}
func (result Result) Document() *authoring.LibraryEditor {
	if result.job == nil {
		return nil
	}
	return result.job.working
}
func (result Result) Revision() uint64 {
	if result.job == nil {
		return 0
	}
	return result.job.revision
}
func (result Result) Error() error      { return result.err }
func (job *Job) Generation() uint64     { return job.generation }
func (documents *Documents) Busy() bool { return documents.active != nil }

func (documents *Documents) Begin(current *authoring.LibraryEditor) (*Job, error) {
	if documents.Busy() {
		return nil, ErrBusy
	}
	if current == nil {
		return nil, ErrDocumentRequired
	}
	documents.generation++
	job := &Job{owner: documents, origin: current, working: current.Clone(), generation: documents.generation, revision: current.Revision()}
	documents.active = job
	return job, nil
}

// Run executes once against detached state. The result is accepted on the event
// loop; the worker never installs it into the active frontend document.
func (job *Job) Run(apply func(*authoring.LibraryEditor) error) Result {
	if !job.ran.CompareAndSwap(false, true) {
		return Result{err: ErrExecuted}
	}
	return Result{job: job, err: apply(job.working)}
}

type Completion struct {
	Document                           *authoring.LibraryEditor
	Ignored, Stale, PublicationChanged bool
	Err                                error
}

// Complete merges irreversible publication facts even when newer edits or a
// source/selection approval reject the authored result. Duplicate/foreign results
// cannot retire another job. There is deliberately no abandon-publication method.
func (documents *Documents) Complete(current *authoring.LibraryEditor, result Result, approval error) Completion {
	completion := Completion{Document: current}
	job := result.job
	if job == nil || job.owner != documents || documents.active != job {
		completion.Ignored = true
		return completion
	}
	documents.active = nil
	if current == nil {
		completion.Err = ErrDocumentRequired
		return completion
	}
	sequence := current.LastPublication().Sequence
	completion.Stale = current != job.origin || current.Revision() != job.revision
	if completion.Stale || approval != nil {
		completion.Err = errors.Join(result.err, approval, current.MergePublication(job.working))
		completion.PublicationChanged = current.LastPublication().Sequence != sequence
		return completion
	}
	// Verify project identity on a detached copy before accepting any operation.
	if err := current.Clone().MergePublication(job.working); err != nil {
		completion.Err = err
		return completion
	}
	completion.Document, completion.Err = job.working, result.err
	completion.PublicationChanged = job.working.LastPublication().Sequence != sequence
	return completion
}
