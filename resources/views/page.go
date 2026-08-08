package views

// Page is the state every screen hands the application layout.
//
// It is embedded rather than repeated. A page declares a struct of its own --
// which is what turns a typo in a field name into a compile error -- and takes
// the chrome from here:
//
//	type InvoicesIndexData struct {
//		Page
//		Invoices []InvoiceRow
//	}
//
// Page implements Layout, the contract layouts/app declares, so embedding it is
// all a page has to do to fit the frame. That is what lets pages with different
// data share one layout, and what lets `aru make:auth` replace the layout
// without touching a single page.
//
// Nothing here is a helper a view reaches for on its own. There is no config(),
// no route() and no auth(): the controller fills these in, so a name that drifts
// is a compile error rather than a blank link -- and a form can never end up
// carrying another session's token under load.
type Page struct {
	// Title is the document title.
	Title string
	// AppName is the brand in the navigation bar.
	AppName string

	// Token is the CSRF token issued for this session. It reaches the markup
	// twice: as the hidden field @csrf writes, and as the hx-headers attribute
	// on <body> that makes every HTMX request carry it.
	Token string

	// Authenticated decides which half of the navigation bar is drawn.
	Authenticated bool
	// UserName is the signed-in person's display name.
	UserName string

	// HomeURL, LoginURL and LogoutURL are where the navigation points. They come
	// from the router, through the controller.
	HomeURL   string
	LoginURL  string
	LogoutURL string
	// RegisterURL is empty when registration is not open, and the layout draws
	// no link then. It moves the "is this route registered" question to the data: an
	// application that never registered the route hides the link rather than
	// linking to a 404.
	RegisterURL string
}

// Compile-time proof that a page embedding Page fits the layout. If the layout
// asks for something else, this line is where the build stops -- in one file,
// naming the contract, rather than in every page at once.
var _ Layout = Page{}

// PageTitle is what the browser tab shows.
func (p Page) PageTitle() string { return p.Title }

// BrandName is the application name, shown in the navigation bar.
func (p Page) BrandName() string { return p.AppName }

// CSRFToken is what @csrf reads to write the hidden field.
//
// It is a method rather than the field itself because the field is also
// interpolated into hx-headers, and one name cannot be both.
func (p Page) CSRFToken() string { return p.Token }

// SignedIn reports whether there is a session behind this render.
func (p Page) SignedIn() bool { return p.Authenticated }

// SignedInName is who the navigation bar greets.
func (p Page) SignedInName() string { return p.UserName }

// HomeLink is where the brand points.
func (p Page) HomeLink() string { return p.HomeURL }

// LoginLink is the sign-in screen.
func (p Page) LoginLink() string { return p.LoginURL }

// LogoutLink is what the sign-out form posts to.
func (p Page) LogoutLink() string { return p.LogoutURL }

// RegisterLink is the sign-up screen, or empty when registration is closed.
func (p Page) RegisterLink() string { return p.RegisterURL }
