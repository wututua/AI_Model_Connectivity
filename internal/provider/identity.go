package provider

import "net/url"

// ModelKey escapes the provider component so the separator has one meaning.
// Common IDs keep their existing keys; callers must treat the result as opaque.
func ModelKey(providerID, model string) string {
	return url.QueryEscape(providerID) + "::" + model
}
