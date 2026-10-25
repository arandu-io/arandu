package services

import "github.com/arandu-io/hesape/pagination"

// mustListing checks a listing declared at package level and returns it. A spec
// that fails is a mistake in the code that wrote it, found when the program
// starts rather than on the first request, so it stops the program.
func mustListing(spec pagination.ListingSpec) *pagination.Listing {
	listing, err := pagination.NewListing(spec)
	if err != nil {
		panic("services: invalid listing: " + err.Error())
	}
	return listing
}
