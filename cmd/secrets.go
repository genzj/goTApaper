package cmd

import (
	"github.com/genzj/goTApaper/channel"
)

// redacted stands in for a secret value wherever configuration is displayed.
//
// API keys are the only secrets goTApaper holds, but they are worth keeping out
// of terminal scrollback, shell history and support pastes. The same sentinel
// is used by the REST API, where a client may write it back unchanged to mean
// "leave this value alone".
const redacted = "********"

// isSecretOption reports whether a channel option key is marked secret by any
// registered channel type.
//
// The check is across all types rather than per type because display code
// often has an option map without knowing which channel produced it, and a key
// that is secret for one provider is secret for all of them in practice.
func isSecretOption(key string) bool {
	for _, t := range channelTypes() {
		if o, ok := t.Schema.Lookup(key); ok && o.Secret {
			return true
		}
	}
	return false
}

// channelTypes lists the registered channel types with their option schemas.
func channelTypes() []channel.TypeInfo {
	return channel.Channels.Types()
}
