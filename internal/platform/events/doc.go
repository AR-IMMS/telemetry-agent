// Package events delivers bounded, process-local advisory events. It accepts
// committed revision hints and passes them to one serial handler; subscribers
// re-read authoritative repository state. The package intentionally does not
// persist events or execute commands.
package events
