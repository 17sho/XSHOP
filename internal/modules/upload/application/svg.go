package application

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// validateSVGSafety retains the existing active-content checks and additionally
// requires a single well-formed passive SVG document with only local references.
func validateSVGSafety(data []byte) error {
	if err := validateSVGActiveContent(data); err != nil {
		return err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth, roots := 0, 0
	var styleText strings.Builder
	inStyle := false
	for {
		tok, err := dec.Token() // Unlike RawToken, validates matching end elements.
		if err == io.EOF {
			if roots != 1 || depth != 0 {
				return fmt.Errorf("SVG 文件必须包含单个完整的 svg 根元素")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("SVG 文件解析失败: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if depth == 0 {
				roots++
				if roots != 1 || name != "svg" {
					return fmt.Errorf("SVG 文件必须包含单个 svg 根元素")
				}
			}
			depth++
			if t.Name.Space != "" && t.Name.Space != "http://www.w3.org/2000/svg" {
				return fmt.Errorf("SVG 文件不允许包含外部命名空间元素")
			}
			// Passive graphics only: no embedded documents or SMIL attribute mutations.
			switch name {
			case "svg", "g", "defs", "symbol", "use", "title", "desc", "metadata", "a", "image",
				"path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "text", "tspan", "textpath",
				"lineargradient", "radialgradient", "stop", "pattern", "clippath", "mask", "marker", "style", "switch",
				"filter", "feblend", "fecolormatrix", "fecomponenttransfer", "fecomposite", "feconvolvematrix",
				"fediffuselighting", "fedisplacementmap", "fedistantlight", "fedropshadow", "feflood", "fefunca", "fefuncb", "fefuncg", "fefuncr",
				"fegaussianblur", "feimage", "femerge", "femergenode", "femorphology", "feoffset", "fepointlight", "fespecularlighting", "fespotlight", "fetile", "feturbulence":
			default:
				return fmt.Errorf("SVG 文件不允许包含元素: %s", name)
			}
			if inStyle {
				return fmt.Errorf("SVG style 不允许嵌套元素")
			}
			if name == "style" {
				inStyle = true
				styleText.Reset()
			}
			for _, attr := range t.Attr {
				aname := strings.ToLower(attr.Name.Local)
				val := strings.TrimSpace(attr.Value)
				if (aname == "href" || aname == "src") && val != "" && !strings.HasPrefix(val, "#") {
					return fmt.Errorf("SVG 文件只允许文档内部引用")
				}
				if attr.Name.Space == "http://www.w3.org/XML/1998/namespace" && aname == "base" {
					return fmt.Errorf("SVG 文件不允许 xml:base")
				}
				if aname == "style" || strings.Contains(strings.ToLower(val), "url") || strings.Contains(val, "\\") {
					if err := validateSVGStyle(val); err != nil {
						return err
					}
				}
			}
		case xml.EndElement:
			depth--
			if strings.EqualFold(t.Name.Local, "style") {
				if err := validateSVGStyle(styleText.String()); err != nil {
					return err
				}
				inStyle = false
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return fmt.Errorf("SVG 根元素外不允许文本")
			}
			if inStyle {
				styleText.Write(t)
			}
		}
	}
}

var svgLocalCSSURL = regexp.MustCompile(`(?i)url\(\s*["']?#[a-z0-9_.:-]+["']?\s*\)`)

func validateSVGStyle(value string) error {
	// Reject escapes/comments/at-rules instead of interpreting a second language.
	// Ordinary styling and local gradient/filter URLs remain supported.
	remaining := strings.ToLower(svgLocalCSSURL.ReplaceAllString(value, ""))
	if strings.ContainsAny(remaining, "\\@") || strings.Contains(remaining, "/*") || strings.Contains(remaining, "url") || strings.Contains(remaining, "expression") {
		return fmt.Errorf("SVG 样式不允许外部引用或动态表达式")
	}
	return nil
}
