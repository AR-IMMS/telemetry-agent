// Package sqlite adapts Fleet enrollment attempt persistence to the Agent's
// shared SQLite database. It stores only immutable request metadata and opaque
// protected-credential references; plaintext invitation, proof, and credential
// values remain in the credential store.
package sqlite