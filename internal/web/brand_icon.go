package web

import (
	"bytes"
	"html"
	"net/url"
	"regexp"
	"strings"
)

var iconLinkPattern = regexp.MustCompile(`(?i)<link\b[^>]*\brel\s*=\s*["'](?:icon|shortcut icon|apple-touch-icon)["'][^>]*>`)

func checkedIcon(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.ContainsAny(raw, "\r\n\\") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	if (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil {
		return raw
	}
	return ""
}
func firstPaintIcon(body []byte, icon string) []byte {
	// Remove old static favicon declarations, including the interim transparent placeholder.
	s := string(body)
	for _, match := range iconLinkPattern.FindAllString(s, -1) {
		s = strings.ReplaceAll(s, match, "")
	}
	href := html.EscapeString(icon)
	tags := `<link rel="icon" href="` + href + `"><link rel="shortcut icon" href="` + href + `"><link rel="apple-touch-icon" href="` + href + `">`
	return bytes.Replace([]byte(s), []byte("</head>"), []byte(tags+"</head>"), 1)
}
