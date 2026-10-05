// Package tailnet is the reviewer's surface on the tailnet: the tsnet node,
// started only for a node the owner enrolled with eyeball tailnet login, and
// the goroutine that owns it.
//
// Nothing here runs in a test against Tailscale. The node is an interface, and
// every behavior is proved against a fake that scripts a node's states. A real
// node runs for the first time when the owner logs it in. See
// docs/architecture.md.
package tailnet
