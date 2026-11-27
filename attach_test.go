package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"os"
	"reflect"
	"strings"
	"testing"
)

func pngOf(t *testing.T, w, h int, noisy bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewPCG(42, 42))
	for y := range h {
		for x := range w {
			c := color.NRGBA{uint8(x), uint8(y), 42, 255}
			if noisy {
				c = color.NRGBA{uint8(r.IntN(256)), uint8(r.IntN(256)), uint8(r.IntN(256)), 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testImage(t *testing.T) Image {
	t.Helper()
	im, err := newImage("towel.png", pngOf(t, 40, 30, false))
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func TestAPastedPNGBecomesAnAttachmentWithAThumbnail(t *testing.T) {
	data := pngOf(t, 400, 100, false)
	im, err := newImage("image.png", data)
	if err != nil {
		t.Fatal(err)
	}
	if im.Type != "image/png" || !bytes.Equal(im.Data, data) {
		t.Fatalf("type %q, %d bytes; want the PNG unchanged", im.Type, len(im.Data))
	}
	if !strings.HasPrefix(im.Thumb, "data:image/png;base64,") || im.H != thumbH || im.W != 4*thumbH {
		t.Fatalf("thumb %.30q at %vx%v, want a PNG data URI %dpx high keeping the aspect", im.Thumb, im.W, im.H, thumbH)
	}
}

func TestANonImageDropIsRefused(t *testing.T) {
	_, err := newImage("guide.txt", []byte("Don't panic."))
	if err == nil || !strings.Contains(err.Error(), "guide.txt is not an image") {
		t.Fatalf("err %v, want a refusal naming the file", err)
	}
}

func TestAnImageOverTheSizeLimitIsRefused(t *testing.T) {
	data := append(pngOf(t, 8, 8, false), make([]byte, maxImageBytes)...)
	_, err := newImage("heart-of-gold.png", data)
	if err == nil || !strings.Contains(err.Error(), "Images up to 20.0 MB") {
		t.Fatalf("err %v, want a size refusal", err)
	}
}

func TestABrokenImageIsRefused(t *testing.T) {
	data := pngOf(t, 8, 8, false)
	_, err := newImage("vogon.png", data[:40])
	if err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("err %v, want a refusal", err)
	}
}

func TestALargeScreenshotIsScaledDownBeforeSending(t *testing.T) {
	im, err := newImage("screenshot.png", pngOf(t, 3000, 1200, true))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(im.Data))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != maxImageSide || cfg.Height != 800 || len(im.Data) > sendImageBytes {
		t.Fatalf("sent %dx%d at %d bytes, want %dx800 under %d bytes", cfg.Width, cfg.Height, len(im.Data), maxImageSide, sendImageBytes)
	}
}

func TestClaudeGetsImageBlocksBeforeTheText(t *testing.T) {
	im := testImage(t)
	msg := claudeUserMessage("What is on the towel?", []Image{im, im})
	content := msg["message"].(map[string]any)["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("content %v, want two images and the text", content)
	}
	src := content[0].(map[string]any)["source"].(map[string]any)
	if content[0].(map[string]any)["type"] != "image" || src["type"] != "base64" || src["media_type"] != "image/png" ||
		src["data"] != base64.StdEncoding.EncodeToString(im.Data) {
		t.Fatalf("first block %v, want a base64 PNG image", content[0])
	}
	if want := map[string]any{"type": "text", "text": "What is on the towel?"}; !reflect.DeepEqual(content[2], want) {
		t.Fatalf("last block %v, want the text", content[2])
	}
	if got := claudeUserMessage("hello", nil)["message"].(map[string]any)["content"]; got != "hello" {
		t.Fatalf("plain message content %v, want the bare string", got)
	}
	if got := claudeUserMessage("", []Image{im})["message"].(map[string]any)["content"].([]any); len(got) != 1 {
		t.Fatalf("image-only content %v, want one image block and no empty text", got)
	}
}

func TestClaudeHistoryShowsImagesTheUserSent(t *testing.T) {
	im := testImage(t)
	content := claudeUserMessage("Look", []Image{im})["message"].(map[string]any)["content"]
	e, ok := claudeReplayUser(content)
	if !ok || e.Text != "Look" || len(e.Images) != 1 || e.Images[0].ID != im.ID {
		t.Fatalf("replayed %+v %v, want the text and the image", e, ok)
	}
	if _, ok := claudeReplayUser([]any{map[string]any{"type": "tool_result"}}); ok {
		t.Fatal("a tool result replayed as a user message")
	}
}

func TestCodexGetsTheImageAsALocalFile(t *testing.T) {
	im := testImage(t)
	dir := t.TempDir()
	in, err := codexInput("Describe it", []Image{im}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(in) != 2 || in[0].(map[string]any)["text"] != "Describe it" || in[1].(map[string]any)["type"] != "localImage" {
		t.Fatalf("input %v, want the text then a localImage", in)
	}
	p := in[1].(map[string]any)["path"].(string)
	if data, err := os.ReadFile(p); err != nil || !bytes.Equal(data, im.Data) || !strings.HasPrefix(p, dir) {
		t.Fatalf("image file %s: %v, want the image bytes in %s", p, err, dir)
	}
	if in, _ := codexInput("", []Image{im}, dir); len(in) != 1 {
		t.Fatalf("image-only input %v, want just the image", in)
	}
}

func TestGeminiGetsAnImageBlock(t *testing.T) {
	im := testImage(t)
	p := geminiPrompt("/help", []Image{im})
	want := []any{
		map[string]any{"type": "text", "text": " /help"},
		map[string]any{"type": "image", "mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(im.Data)},
	}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("prompt %v, want text then image", p)
	}
	e, ok := geminiReplayUser(want[1])
	if !ok || len(e.Images) != 1 || e.Images[0].ID != im.ID {
		t.Fatalf("replayed %+v %v, want the image", e, ok)
	}
}

func TestImagesSentMidTurnWaitWithTheirMessage(t *testing.T) {
	a, d := busyAgent(t)
	im := testImage(t)
	a.SendImages("What is this?", []Image{im})
	a.SendImages("", []Image{im, im})

	s := a.Snapshot()
	if len(s.Queued) != 2 || len(s.Queued[0].Images) != 1 || len(s.Queued[1].Images) != 2 {
		t.Fatalf("pending %+v, want both messages holding their images", s.Queued)
	}
	endTurn(a)
	wantDelivered(t, d, "Find the Ultimate Question", "What is this?")
	d.mu.Lock()
	sent := d.imgs[1]
	d.mu.Unlock()
	if len(sent) != 3 {
		t.Fatalf("delivered %d images, want all three", len(sent))
	}
	s = waitFor(t, a, "pending cleared", func(s Snapshot) bool { return len(s.Queued) == 0 })
	var shown []int
	for _, e := range s.Entries {
		if e.Kind == EntryUser {
			shown = append(shown, len(e.Images))
		}
	}
	if !reflect.DeepEqual(shown, []int{0, 1, 2}) {
		t.Fatalf("transcript image counts %v, want each message with its own images", shown)
	}
}

func TestEditingAwayTheTextOfAPendingImageKeepsTheImage(t *testing.T) {
	a, _ := busyAgent(t)
	a.SendImages("Look", []Image{testImage(t)})
	id := a.Snapshot().Queued[0].ID
	if _, ok := a.BeginEdit(id); !ok {
		t.Fatal("could not edit")
	}
	a.EndEdit(id, "", true)
	if q := a.Snapshot().Queued; len(q) != 1 || q[0].Text != "" || len(q[0].Images) != 1 {
		t.Fatalf("pending %+v, want the image kept without text", q)
	}
}
