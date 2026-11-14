// Package nats adapts authenticated JetStream command consumption and result
// publication to Operations ports. It owns broker reconnection, explicit ACK,
// and PubAck handling; it does not own command policy, SQLite transactions, or
// Core business authorization.
package nats
