package dredge

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

type PageMeta struct {
	Title       string
	Description string
}

func ScrapeMetadata(body io.Reader) PageMeta {
	var meta PageMeta
	z := html.NewTokenizer(body)
	var inTitle bool

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return meta
		case html.StartTagToken:
			tn, hasAttr := z.TagName()
			if string(tn) == "title" {
				inTitle = true
			}
			if string(tn) == "meta" && hasAttr {
				var name, property, content string
				for {
					key, val, more := z.TagAttr()
					k := strings.ToLower(string(key))
					if k == "name" {
						name = strings.ToLower(string(val))
					}
					if k == "property" {
						property = strings.ToLower(string(val))
					}
					if k == "content" {
						content = string(val)
					}
					if !more {
						break
					}
				}
				if content != "" {
					if name == "description" && meta.Description == "" {
						meta.Description = content
					}
					if property == "og:description" && meta.Description == "" {
						meta.Description = content
					}
					if property == "og:title" && meta.Title == "" {
						meta.Title = content
					}
				}
			}
		case html.TextToken:
			if inTitle {
				meta.Title = strings.TrimSpace(string(z.Text()))
				inTitle = false
			}
		case html.EndTagToken:
			tn, _ := z.TagName()
			if string(tn) == "title" {
				inTitle = false
			}
		}
	}
}
