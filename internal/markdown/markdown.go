// Package markdown converts rendered HTML into Markdown, mirroring the
// Turndown configuration used by the Mellowtel SDKs (ATX headings, fenced code
// blocks, "*" bullet markers).
package markdown

import (
	"fmt"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
)

// Converter wraps a configured html-to-markdown converter. It is safe for
// concurrent use.
type Converter struct {
	conv *md.Converter
}

// New builds a Converter configured to match the SDK output style.
func New() *Converter {
	c := md.NewConverter("", true, &md.Options{
		HeadingStyle: "atx",
		CodeBlockStyle: "fenced",
		BulletListMarker: "*",
	})
	c.Use(plugin.GitHubFlavored())
	return &Converter{conv: c}
}

// Convert turns an HTML document into Markdown.
func (c *Converter) Convert(html string) (string, error) {
	out, err := c.conv.ConvertString(html)
	if err != nil {
		return "", fmt.Errorf("convert html to markdown: %w", err)
	}
	return out, nil
}
