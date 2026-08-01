package markup_test

import (
	"context"
	"testing"
	"unicode/utf8"

	"github.com/timonwong/jiro/internal/markup"
)

func FuzzToJFMNeverPanicsAndReturnsValidUTF8(f *testing.F) {
	for _, seed := range [][]byte{nil, []byte("h1. Release"), []byte("{panel}\n* item\n{panel}"), {0xff, 0xfe, 'x'}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		result, err := markup.ToJFM(context.Background(), string(input))
		if err != nil {
			t.Fatal(err)
		}
		if !utf8.ValidString(result.Markdown) {
			t.Fatalf("invalid UTF-8 output %q", result.Markdown)
		}
	})
}

func FuzzFromJFMNeverPanicsAndReturnsValidUTF8(f *testing.F) {
	for _, seed := range [][]byte{nil, []byte("# Release"), []byte(":::panel\n- item\n:::"), {0xff, 0xfe, 'x'}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		result, err := markup.FromJFM(context.Background(), string(input))
		if err != nil {
			t.Fatal(err)
		}
		if !utf8.ValidString(result.Markup) {
			t.Fatalf("invalid UTF-8 output %q", result.Markup)
		}
	})
}
