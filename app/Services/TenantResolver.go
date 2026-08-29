package services

import "net/http"

// TenantResolver decides which tenant an unauthenticated request belongs to.
type TenantResolver func(*http.Request) string

// FixedTenant returns a resolver for a single configured tenant.
func FixedTenant(tenant string) TenantResolver {
	return func(*http.Request) string { return tenant }
}
