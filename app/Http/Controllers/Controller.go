// Package controllers holds this application's controllers.
//
// The directory is app/Http/Controllers, in CamelCase, which is where people
// look for it. The package name follows Go and stays lowercase.
//
// A controller reads the request, calls a service and renders. It never reaches
// the database: a controller holding a repository is a controller that skipped
// the service, and therefore skipped the policy, and `aru doctor` refuses it.
//
// Nor does it answer a rejected form. The service returns validation.Errors,
// the action returns them unchanged, and the router answers: a page goes back
// to the form with the messages and what was typed, and a client that asked
// for JSON gets a 422 problem document with the messages by field.
package controllers

// Controller is the type every controller in this directory embeds.
//
// It carries no dependencies and no methods, and that is deliberate. A base
// controller that grows authorize(), validate() and dispatch() grows them
// because there is nowhere else to put them, and each one hides a collaborator
// or a second way to answer something the router already answers. A
// controller's collaborators are fields it declares and the constructor sets.
type Controller struct{}
