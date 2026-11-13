package agenthealth

import (
	"regexp"
	"strings"
	"unicode"
)

const maxDiagnosticTextLength = 512

var (
	credentialAssignment = regexp.MustCompile(`(?i)\b(password|passwd|passphrase|token|secret|api[_-]?key|private[_-]?key|client[_-]?key|authorization)(\s*[:=]\s*)(?:bearer\s+)?("[^"]*"|'[^']*'|[^\s,;]+)`)
	pemPrivateKey        = regexp.MustCompile(`(?is)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)
	urlUserInfo          = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/@\s]+@`)
	bearerCredential     = regexp.MustCompile(`(?i)\bbearer\s+[^\s,;]+`)
)

// sanitizeError strips common credential forms and bounds diagnostic text before exposure.
func sanitizeError(value string) string {
	value = stripControlCharacters(value)
	value = strings.TrimSpace(value)
	value = urlUserInfo.ReplaceAllString(value, `${1}[REDACTED]@`)
	value = pemPrivateKey.ReplaceAllString(value, "[REDACTED PRIVATE KEY]")
	value = credentialAssignment.ReplaceAllString(value, `${1}${2}[REDACTED]`)
	value = bearerCredential.ReplaceAllString(value, `Bearer [REDACTED]`)
	if len(value) > maxDiagnosticTextLength {
		value = value[:maxDiagnosticTextLength]
	}
	return value
}

// boundDiagnosticText caps externally sourced diagnostic identifiers.
func boundDiagnosticText(value string) string {
	value = stripControlCharacters(value)
	value = strings.TrimSpace(value)
	if len(value) > maxDiagnosticTextLength {
		return value[:maxDiagnosticTextLength]
	}
	return value
}

func stripControlCharacters(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
}
