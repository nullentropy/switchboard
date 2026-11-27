package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	maxImageBytes  = 20 << 20
	sendImageBytes = 3<<20 + 3<<18
	maxImageSide   = 2000
	maxImages      = 10
	thumbH         = 48
)

type Image struct {
	ID    string
	Type  string
	Data  []byte
	Thumb string
	W, H  float64
}

func (im Image) String() string { return im.ID }

var imageExt = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}

func megabytes(n int) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

func newImage(name string, data []byte) (Image, error) {
	if name == "" {
		name = "The image"
	}
	typ := http.DetectContentType(data)
	if _, ok := imageExt[typ]; !ok {
		return Image{}, fmt.Errorf("%s is not an image. Only PNG, JPEG, GIF and WebP images can be attached.", name)
	}
	if len(data) > maxImageBytes {
		return Image{}, fmt.Errorf("%s is %s. Images up to %s can be attached.", name, megabytes(len(data)), megabytes(maxImageBytes))
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Image{}, fmt.Errorf("%s could not be read as an image.", name)
	}
	b := img.Bounds()
	if len(data) > sendImageBytes || b.Dx() > maxImageSide || b.Dy() > maxImageSide {
		if data, typ, err = shrink(img); err != nil {
			return Image{}, fmt.Errorf("%s is too large to send, even scaled down.", name)
		}
	}
	thumb := scaleTo(img, 2*thumbH*4, 2*thumbH)
	var buf bytes.Buffer
	if err := png.Encode(&buf, thumb); err != nil {
		return Image{}, err
	}
	sum := sha256.Sum256(data)
	tb := thumb.Bounds()
	return Image{
		ID:    hex.EncodeToString(sum[:8]),
		Type:  typ,
		Data:  data,
		Thumb: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
		W:     float64(tb.Dx()) / 2,
		H:     float64(tb.Dy()) / 2,
	}, nil
}

func scaleTo(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW && h <= maxH {
		return img
	}
	k := min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	dst := image.NewNRGBA(image.Rect(0, 0, max(1, int(float64(w)*k)), max(1, int(float64(h)*k))))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

func shrink(img image.Image) ([]byte, string, error) {
	small := scaleTo(img, maxImageSide, maxImageSide)
	var buf bytes.Buffer
	if err := png.Encode(&buf, small); err == nil && buf.Len() <= sendImageBytes {
		return buf.Bytes(), "image/png", nil
	}
	flat := image.NewRGBA(small.Bounds())
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), small, small.Bounds().Min, draw.Over)
	buf.Reset()
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: 85}); err != nil {
		return nil, "", err
	}
	if buf.Len() > sendImageBytes {
		return nil, "", fmt.Errorf("image is %s after scaling", megabytes(buf.Len()))
	}
	return buf.Bytes(), "image/jpeg", nil
}

func claudeUserMessage(text string, imgs []Image) map[string]any {
	var content any = text
	if len(imgs) > 0 {
		var blocks []any
		for _, im := range imgs {
			blocks = append(blocks, map[string]any{"type": "image", "source": map[string]any{
				"type": "base64", "media_type": im.Type, "data": base64.StdEncoding.EncodeToString(im.Data)}})
		}
		if text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": text})
		}
		content = blocks
	}
	return map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}}
}

func claudeReplayUser(content any) (Entry, bool) {
	blocks, _ := content.([]any)
	var texts []string
	var imgs []Image
	for _, b := range blocks {
		blk, _ := b.(map[string]any)
		switch blk["type"] {
		case "text":
			if t, _ := blk["text"].(string); t != "" {
				texts = append(texts, t)
			}
		case "image":
			src, _ := blk["source"].(map[string]any)
			s, _ := src["data"].(string)
			if data, err := base64.StdEncoding.DecodeString(s); err == nil {
				if im, err := newImage("", data); err == nil {
					imgs = append(imgs, im)
				}
			}
		default:
			return Entry{}, false
		}
	}
	text := strings.Join(texts, "\n\n")
	if len(imgs) == 0 || strings.HasPrefix(text, "<") {
		return Entry{}, false
	}
	return Entry{Kind: EntryUser, Text: text, Images: imgs}, true
}

func imageDir() string { return filepath.Join(os.TempDir(), "switchboard-images") }

func imageFile(dir string, im Image) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, im.ID+imageExt[im.Type])
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	if err := os.WriteFile(p, im.Data, 0o600); err != nil {
		return "", err
	}
	return p, nil
}

func codexInput(text string, imgs []Image, dir string) ([]any, error) {
	var in []any
	if text != "" || len(imgs) == 0 {
		in = append(in, map[string]any{"type": "text", "text": text, "text_elements": []any{}})
	}
	for _, im := range imgs {
		p, err := imageFile(dir, im)
		if err != nil {
			return nil, fmt.Errorf("save an image for Codex: %w", err)
		}
		in = append(in, map[string]any{"type": "localImage", "path": p})
	}
	return in, nil
}

func geminiPrompt(text string, imgs []Image) []any {
	if strings.HasPrefix(text, "/") || strings.HasPrefix(text, "$") {
		text = " " + text
	}
	var out []any
	if text != "" || len(imgs) == 0 {
		out = append(out, map[string]any{"type": "text", "text": text})
	}
	for _, im := range imgs {
		out = append(out, map[string]any{"type": "image", "mimeType": im.Type, "data": base64.StdEncoding.EncodeToString(im.Data)})
	}
	return out
}

func userLine(e Entry) string {
	switch n := len(e.Images); {
	case n == 1:
		return strings.TrimSpace(e.Text + " [1 image]")
	case n > 1:
		return strings.TrimSpace(fmt.Sprintf("%s [%d images]", e.Text, n))
	}
	return e.Text
}

func geminiReplayUser(content any) (Entry, bool) {
	c, _ := content.(map[string]any)
	switch c["type"] {
	case "text":
		if t, _ := c["text"].(string); t != "" {
			return Entry{Kind: EntryUser, Text: t}, true
		}
	case "image":
		s, _ := c["data"].(string)
		if data, err := base64.StdEncoding.DecodeString(s); err == nil {
			if im, err := newImage("", data); err == nil {
				return Entry{Kind: EntryUser, Images: []Image{im}}, true
			}
		}
	}
	return Entry{}, false
}
