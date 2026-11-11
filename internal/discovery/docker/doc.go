// Package docker adapts the Docker Engine API to the provider-neutral discovery
// contract. It reads container summaries only and never collects secrets,
// environment variables, command lines, volume contents, or Integration config.
package docker
