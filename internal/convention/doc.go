// Package convention holds this repository to the rules it can check
// mechanically, rather than trusting that they are followed.
//
// The reason is in docs/conventions.md: a convention that is only written down
// gets violated. Each check corresponds to one entry there marked "Enforced by
// internal/convention", and every check has a test proving it flags a violating
// fixture. A gate nobody has watched fail is not a gate.
//
// Fixtures live under testdata/, which the go tool ignores, so a file that
// deliberately breaks a rule never has to compile.
package convention
