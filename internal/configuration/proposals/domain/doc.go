// Package domain owns the proposal lifecycle model, stable identity, and
// validation invariants. It accepts normalized proposal data and reports
// validation errors; it does not read Inventory, persist proposals, or approve
// desired-state changes.
package domain
