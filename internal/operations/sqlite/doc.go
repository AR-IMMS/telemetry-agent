// Package sqlite implements the Operations execution journal and result outbox
// on the Agent's existing operational database. It owns SQL transactions and
// bounded durable payloads; it does not own dispatch policy or NATS delivery.
package sqlite
