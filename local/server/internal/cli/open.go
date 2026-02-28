package cli

import "fmt"

// Opener abstracts opening a URL in the user's browser.
type Opener interface {
	Open(url string) error
}

// XDGOpener opens URLs using xdg-open.
type XDGOpener struct {
	Cmd Commander
}

func (o XDGOpener) Open(url string) error {
	return o.Cmd.Run("xdg-open", url)
}

// RunOpen opens the buildermark URL in the user's browser.
func RunOpen(opener Opener, addr string) error {
	url := fmt.Sprintf("http://localhost%s", addr)
	return opener.Open(url)
}
