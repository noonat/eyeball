// Package view holds the JSON the surface's API sends, one type for each view.
//
// web/src/api/types.ts is generated from this package by tool/apitypes, and
// make check fails when the committed file differs. Every field's Go type has
// to map exactly to TypeScript: an interface, an int that can pass 2^53 or a
// map without string keys fails the generator. An id is an int64 sent as a
// string through the ",string" tag, so no id loses digits in the browser.
package view
