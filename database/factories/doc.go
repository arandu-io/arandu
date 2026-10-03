// Package factories builds rows with plausible values, for seeders and tests.
//
// A factory makes a row and stores nothing until Create is called, and Create
// takes the Grant every write takes: the tenant comes off it, and a factory is
// no way around the policy that guards the table.
//
// Each <Entity>Factory.go is rendered by aru model:build from the entity it
// builds. Everything outside its custom block is rewritten on the next build;
// the default state and the named states inside it are yours.
package factories
