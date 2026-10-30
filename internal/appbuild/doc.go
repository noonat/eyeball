// Package appbuild holds the esbuild options for the app's bundle, so the two
// builds that must agree take them from one place: the vendor step bundles
// from web/node_modules, and the binary bundles from the files it embeds.
//
// The bundles must be byte for byte the same, since that is what proves the
// vendored set is complete. See docs/architecture.md, The client code.
package appbuild
