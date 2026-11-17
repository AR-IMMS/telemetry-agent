// Package proposals preserves the pre-layering import path for proposal
// models and use cases. The domain and application packages own proposal
// validation and workflow decisions; this package only forwards those APIs
// while SQLite and CLI callers migrate to the owner packages.
package proposals
