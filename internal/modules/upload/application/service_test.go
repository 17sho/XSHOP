package application

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/modules/upload/contract"
)

type memoryStore struct {
	data []byte
}

func (s *memoryStore) Save(input contract.StoreInput) (string, error) {
	data, err := io.ReadAll(input.Source)
	if err != nil {
		return "", err
	}
	s.data = data
	return "/uploads/" + input.Scene + "/" + input.Year + "/" + input.Month + "/" + input.Filename, nil
}

func fixtureZIP(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, err := w.Create("fixture.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("fixture attachment")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestUploadSVGRequiresPassiveWellFormedDocument(t *testing.T) {
	for _, data := range []string{
		`<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body>fixture</body></html>`,
		`<svg xmlns="http://www.w3.org/1999/xhtml"><iframe src="https://example.invalid"/></svg>`,
		`<svg><g></svg>`,
		`<svg/><svg/>`,
		`<svg/><html/>`,
		`<svg><image href="https://example.invalid/image.png"/></svg>`,
		`<svg><use xmlns:xlink="http://www.w3.org/1999/xlink" xlink:href="//example.invalid/a.svg#x"/></svg>`,
		`<svg><animate attributeName="href" values="javascript:alert(1)"/></svg>`,
		`<svg><set attributeName="href" to="https://example.invalid"/></svg>`,
		`<svg><style>@import 'https://example.invalid/style.css';</style></svg>`,
		`<svg><rect style="fill:url(https://example.invalid/a.svg#x)"/></svg>`,
		`<svg><style>.x{fill:u\72l(https://example.invalid/a)}</style></svg>`,
	} {
		t.Run(data, func(t *testing.T) {
			store := &memoryStore{}
			svc := NewService(Policy{MaxSize: 1024 * 1024}, store)
			if _, err := svc.SaveFileWithMeta(createMultipartFile(t, "fixture.svg", []byte(data)), "telegram"); err == nil {
				t.Fatal("accepted unsafe SVG")
			}
			if len(store.data) != 0 {
				t.Fatal("unsafe SVG reached storage")
			}
		})
	}
	safe := `<svg xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="g"><stop offset="0" stop-color="red"/></linearGradient></defs><style>.x{fill:url(#g)}</style><rect class="x" width="2" height="3"/><use href="#g"/></svg>`
	svc := NewService(Policy{MaxSize: 1024 * 1024}, &memoryStore{})
	if _, err := svc.SaveFileWithMeta(createMultipartFile(t, "fixture.svg", []byte(safe)), "telegram"); err != nil {
		t.Fatal(err)
	}
}

func TestUploadRejectsActiveAndUnknownContent(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"page.html", "<!doctype html><html><script>alert(1)</script></html>"},
		{"page.xhtml", `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><script>alert(1)</script></html>`},
		{"code.js", "alert(1)"},
		{"code.mjs", "export default 1"},
		{"style.css", "body{background:red}"},
		{"hidden.zip", "<!doctype html><html><script>alert(1)</script></html>"},
		{"hidden.pdf", `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"/>`},
		{"hidden.gz", "alert(1)"},
		{"hidden.txt", "alert(1)"},
		{"hidden.bin", "\x00\x01\x02unknown"},
		{"hidden.png", "<html>fixture</html>"},
		{"bad.svg", `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryStore{}
			// Even an empty operator allowlist must not authorize arbitrary files.
			svc := NewService(Policy{MaxSize: 1024 * 1024}, store)
			for _, scene := range []string{"telegram", "common"} {
				if _, err := svc.SaveFileWithMeta(createMultipartFile(t, tc.name, []byte(tc.data)), scene); err == nil {
					t.Errorf("%s accepted active/unknown upload", scene)
				}
				if len(store.data) != 0 {
					t.Error("rejected content reached storage")
				}
			}
		})
	}
}

func TestUploadPreservesImagesAndKnownTelegramAttachments(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	// A locally generated 2x3 lossless WebP fixture, not an external asset.
	webpData, err := base64.StdEncoding.DecodeString("UklGRhwAAABXRUJQVlA4TA8AAAAvAYAAAAcQ/Y/+ByKi/wEA")
	if err != nil {
		t.Fatal(err)
	}
	var pngData, jpegData, gifData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegData, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(&gifData, img, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mime string
		data       []byte
	}{
		{"fixture.png", "image/png", pngData.Bytes()},
		{"fixture.JPG", "image/jpeg", jpegData.Bytes()},
		{"fixture.gif", "image/gif", gifData.Bytes()},
		{"fixture.webp", "image/webp", webpData},
		{"fixture.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="2" height="3"/></svg>`)},
		{"fixture.zip", "application/zip", fixtureZIP(t)},
		{"fixture.pdf", "application/pdf", []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryStore{}
			svc := NewService(Policy{MaxSize: 1024 * 1024, AllowedExtensions: []string{"png", "jpg", "gif", "webp", "svg"}, AllowedTypes: []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/svg+xml"}}, store)
			result, err := svc.SaveFileWithMeta(createMultipartFile(t, tc.name, tc.data), "telegram")
			if err != nil {
				t.Fatal(err)
			}
			if result.MimeType != tc.mime || !bytes.Equal(store.data, tc.data) {
				t.Fatalf("changed media: %#v", result)
			}
			if tc.mime == "application/zip" || tc.mime == "application/pdf" {
				if _, err := svc.SaveFileWithMeta(createMultipartFile(t, tc.name, tc.data), "common"); err == nil {
					t.Fatal("attachment bypassed image-only normal scene policy")
				}
			} else {
				if _, err := svc.SaveFileWithMeta(createMultipartFile(t, tc.name, tc.data), "common"); err != nil {
					t.Fatalf("normal image rejected: %v", err)
				}
			}
		})
	}
}

// webPFixture builds bounded container fixtures, not decoded image payloads.
func webPFixture(kind string, payload []byte) []byte {
	data := make([]byte, 20+len(payload)+len(payload)%2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:], "WEBP")
	copy(data[12:], kind)
	binary.LittleEndian.PutUint32(data[16:20], uint32(len(payload)))
	copy(data[20:], payload)
	return data
}

type boundedWebPReader struct {
	*bytes.Reader
	maxRead int
}

func (r *boundedWebPReader) Read(p []byte) (int, error) {
	if len(p) > r.maxRead {
		r.maxRead = len(p)
	}
	return r.Reader.Read(p)
}

func TestWebPDimensionsReadOnlySmallHeaders(t *testing.T) {
	for _, kind := range []string{"VP8 ", "VP8L", "VP8X"} {
		t.Run(kind, func(t *testing.T) {
			// Large enough to catch chunk-sized reads, small enough for a safe RED run.
			payload := make([]byte, 64*1024)
			switch kind {
			case "VP8 ":
				copy(payload[3:], []byte{0x9d, 0x01, 0x2a, 2, 0, 3, 0})
			case "VP8L":
				payload[0] = 0x2f
				binary.LittleEndian.PutUint32(payload[1:], 1|(2<<14))
			case "VP8X":
				payload[4], payload[7] = 1, 2
			}
			data := webPFixture(kind, payload)
			// Exercise bounded skipping, including odd-sized chunk padding.
			unknown := webPFixture("JUNK", make([]byte, 4097))[12:]
			data = append(append(append([]byte{}, data[:12]...), unknown...), data[12:]...)
			binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
			r := &boundedWebPReader{Reader: bytes.NewReader(data)}
			w, h, err := decodeWebPDimensions(r)
			if err != nil || w != 2 || h != 3 {
				t.Fatalf("dimensions=%dx%d error=%v", w, h, err)
			}
			if r.maxRead > 12 {
				t.Fatalf("read buffer size %d depends on chunk payload", r.maxRead)
			}
		})
	}
}

func TestUploadWebPRejectsInvalidContainerBounds(t *testing.T) {
	for _, corruption := range []string{"riff-short", "riff-long", "chunk-outside-riff", "chunk-oversized", "chunk-truncated", "missing-padding", "header-short", "unknown-oversized"} {
		t.Run(corruption, func(t *testing.T) {
			data := webPFixture("VP8L", []byte{0x2f, 1, 0x80, 0, 0})
			switch corruption {
			case "riff-short":
				binary.LittleEndian.PutUint32(data[4:8], 3)
			case "riff-long":
				binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)))
			case "chunk-outside-riff":
				binary.LittleEndian.PutUint32(data[4:8], 12)
			case "chunk-oversized", "unknown-oversized":
				binary.LittleEndian.PutUint32(data[16:20], 64*1024)
				if corruption == "unknown-oversized" {
					copy(data[12:16], "JUNK")
				}
			case "chunk-truncated":
				data = data[:len(data)-2]
			case "missing-padding":
				data = data[:len(data)-1]
				binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
			case "header-short":
				data = webPFixture("VP8L", []byte{0x2f, 1, 0x80, 0})
			}
			store := &memoryStore{}
			svc := NewService(Policy{MaxSize: 1024 * 1024}, store)
			if _, err := svc.SaveFileWithMeta(createMultipartFile(t, "fixture.webp", data), "telegram"); err == nil {
				t.Error("accepted invalid WebP container bounds")
			}
			if len(store.data) != 0 {
				t.Error("invalid WebP reached storage")
			}
		})
	}
}

func TestUploadServiceSaveFileAllowsArchiveForTelegramScene(t *testing.T) {
	policy := Policy{
		MaxSize:           10 * 1024 * 1024,
		AllowedTypes:      []string{"image/jpeg", "image/png"},
		AllowedExtensions: []string{".jpg", ".png"},
	}
	store := &memoryStore{}
	service := NewService(policy, store)
	zipData := fixtureZIP(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "demo.zip")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := part.Write(zipData); err != nil {
		t.Fatalf("write form content failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	reader := multipart.NewReader(&body, writer.Boundary())
	form, err := reader.ReadForm(1024 * 1024)
	if err != nil {
		t.Fatalf("read form failed: %v", err)
	}
	files := form.File["file"]
	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}

	result, err := service.SaveFileWithMeta(files[0], "telegram")
	if err != nil {
		t.Fatalf("save file failed: %v", err)
	}
	savedPath := result.URL
	if filepath.Ext(savedPath) != ".zip" {
		t.Fatalf("expected .zip saved path, got %s", savedPath)
	}
	if !bytes.Equal(store.data, zipData) {
		t.Fatalf("stored content got %q", store.data)
	}
}

func TestUploadServiceSaveFileSVG(t *testing.T) {
	policy := Policy{
		MaxSize:           10 * 1024 * 1024,
		AllowedTypes:      []string{"image/jpeg", "image/png", "image/svg+xml"},
		AllowedExtensions: []string{".jpg", ".png", ".svg"},
	}
	store := &memoryStore{}
	svc := NewService(policy, store)

	safeSVG := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40" fill="red"/></svg>`

	t.Run("safe SVG upload succeeds", func(t *testing.T) {
		fh := createMultipartFile(t, "icon.svg", []byte(safeSVG))
		result, err := svc.SaveFileWithMeta(fh, "common")
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		path := result.URL
		if filepath.Ext(path) != ".svg" {
			t.Fatalf("expected .svg extension, got %s", path)
		}
		if string(store.data) != safeSVG {
			t.Fatalf("stored SVG mismatch: %q", store.data)
		}
	})

	t.Run("SVG with script tag is rejected", func(t *testing.T) {
		malicious := `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`
		fh := createMultipartFile(t, "bad.svg", []byte(malicious))
		_, err := svc.SaveFileWithMeta(fh, "common")
		if err == nil {
			t.Fatal("expected error for SVG with script tag")
		}
		if !strings.Contains(err.Error(), "<script>") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("SVG with event handler is rejected", func(t *testing.T) {
		malicious := `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><circle cx="50" cy="50" r="40"/></svg>`
		fh := createMultipartFile(t, "bad2.svg", []byte(malicious))
		_, err := svc.SaveFileWithMeta(fh, "common")
		if err == nil {
			t.Fatal("expected error for SVG with event handler")
		}
		if !strings.Contains(err.Error(), "onload") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("SVG with javascript protocol is rejected", func(t *testing.T) {
		malicious := `<svg xmlns="http://www.w3.org/2000/svg"><a href="javascript:alert(1)"><circle cx="50" cy="50" r="40"/></a></svg>`
		fh := createMultipartFile(t, "bad3.svg", []byte(malicious))
		_, err := svc.SaveFileWithMeta(fh, "common")
		if err == nil {
			t.Fatal("expected error for SVG with javascript protocol")
		}
	})

	t.Run("SVG with foreignObject is rejected", func(t *testing.T) {
		malicious := `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><body xmlns="http://www.w3.org/1999/xhtml"><div>hello</div></body></foreignObject></svg>`
		fh := createMultipartFile(t, "bad4.svg", []byte(malicious))
		_, err := svc.SaveFileWithMeta(fh, "common")
		if err == nil {
			t.Fatal("expected error for SVG with foreignObject")
		}
	})

	t.Run("SVG with XML declaration", func(t *testing.T) {
		xmlSVG := `<?xml version="1.0" encoding="UTF-8"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><rect width="100" height="100" fill="blue"/></svg>`
		fh := createMultipartFile(t, "xml.svg", []byte(xmlSVG))
		result, err := svc.SaveFileWithMeta(fh, "common")
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		path := result.URL
		if filepath.Ext(path) != ".svg" {
			t.Fatalf("expected .svg extension, got %s", path)
		}
	})
}

func TestIsSVGContent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"svg tag", `<svg xmlns="http://www.w3.org/2000/svg"></svg>`, true},
		{"xml declaration", `<?xml version="1.0"?><svg></svg>`, true},
		{"not svg", `<html><body></body></html>`, false},
		{"plain text", `hello world`, false},
		{"empty", ``, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSVGContent([]byte(tt.input))
			if got != tt.expect {
				t.Errorf("isSVGContent(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestValidateSVGSafety(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"safe svg", `<svg xmlns="http://www.w3.org/2000/svg"><circle cx="50" cy="50" r="40"/></svg>`, false},
		{"script tag", `<svg><script>alert(1)</script></svg>`, true},
		{"onclick", `<svg onclick="alert(1)"></svg>`, true},
		{"javascript href", `<svg><a href="javascript:void(0)"></a></svg>`, true},
		{"data uri html", `<svg><image href="data:text/html,<h1>hi</h1>"/></svg>`, true},
		{"foreignObject", `<svg><foreignObject></foreignObject></svg>`, true},
		{"onload tab before equals", "<svg xmlns=\"http://www.w3.org/2000/svg\" onload\t=\"alert(1)\"></svg>", true},
		{"onload newline before equals", "<svg onload\n=\"alert(1)\"></svg>", true},
		{"onload double space", `<svg onload  ="alert(1)"></svg>`, true},
		{"unlisted event attr onbegin", `<svg><set attributeName="x" onbegin="alert(1)"/></svg>`, true},
		{"entity encoded javascript", `<svg><a href="&#106;avascript:alert(1)"></a></svg>`, true},
		{"xlink href javascript", `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><a xlink:href="javascript:alert(1)"></a></svg>`, true},
		{"xml-stylesheet pi", `<?xml-stylesheet href="http://evil/x.css"?><svg></svg>`, true},
		{"entity declaration", `<!DOCTYPE svg [<!ENTITY x "y">]><svg>&x;</svg>`, true},
		{"malformed xml", `<svg onload="alert(1)"`, true},
		{"safe with doctype and style", `<?xml version="1.0"?><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg xmlns="http://www.w3.org/2000/svg"><style>.a{fill:red}</style><rect class="a" width="10" height="10"/></svg>`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSVGSafety([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSVGSafety() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// createMultipartFile 辅助函数：创建模拟的 multipart 文件
func createMultipartFile(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form content failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}
	reader := multipart.NewReader(&body, writer.Boundary())
	form, err := reader.ReadForm(1024 * 1024)
	if err != nil {
		t.Fatalf("read form failed: %v", err)
	}
	files := form.File["file"]
	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}
	return files[0]
}
