package public

import "encoding/json"

// Identity holds the portal's shared browser and social presentation.
type Identity struct {
	Name         string
	Manifest     string
	AppleIcon    string
	ThemeColor   string
	SocialImage  string
	SocialWidth  int
	SocialHeight int
	SocialType   string
	SocialAlt    string
	TwitterCard  string
}

var identity = loadIdentity()

func loadIdentity() Identity {
	body, err := files.ReadFile("site.webmanifest")
	if err != nil {
		panic("public: missing web manifest: " + err.Error())
	}
	var manifest struct {
		Name       string `json:"name"`
		ThemeColor string `json:"theme_color"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		panic("public: invalid web manifest: " + err.Error())
	}
	return Identity{
		Name: manifest.Name, Manifest: "/site.webmanifest", AppleIcon: "/apple-touch-icon.png",
		ThemeColor:  manifest.ThemeColor,
		SocialImage: "/social-cover.png", SocialWidth: 1200, SocialHeight: 630,
		SocialType: "image/png", SocialAlt: manifest.Name, TwitterCard: "summary_large_image",
	}
}

// Branding returns a copy of the shared identity, without request-specific state.
func Branding() Identity { return identity }

// SocialCover returns the shared composition with a localized description.
func SocialCover(locale string) string {
	switch locale {
	case "pt":
		return "/social-cover-pt.png"
	case "es":
		return "/social-cover-es.png"
	default:
		return identity.SocialImage
	}
}
