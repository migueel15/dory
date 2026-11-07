package service

import (
	"errors"
	"testing"
)

func TestResolveTitleSource(t *testing.T) {
	pending := &Item{Title: "borrador", TitleSource: TitlePending}
	ai := &Item{Title: "Título IA", TitleSource: TitleAI}
	fallback := &Item{Title: "borrador", TitleSource: TitleFallback}
	user := &Item{Title: "mío", TitleSource: TitleUser}
	cases := []struct {
		name    string
		current *Item
		in      Item
		want    string
		invalid bool
	}{
		{"new defaults to user", nil, Item{Title: "x"}, TitleUser, false},
		{"new pending", nil, Item{Title: "x", TitleSource: TitlePending}, TitlePending, false},
		{"new ai rejected", nil, Item{Title: "x", TitleSource: TitleAI}, "", true},
		{"ai fills pending", pending, Item{Title: "Corto", TitleSource: TitleAI}, TitleAI, false},
		{"ai cannot overwrite user", user, Item{Title: "otro", TitleSource: TitleAI}, "", true},
		{"ai cannot rewrite ai", ai, Item{Title: "otro", TitleSource: TitleAI}, "", true},
		{"user edit of ai title", ai, Item{Title: "editado", TitleSource: TitleAI}, "", true},
		{"user edit sends user", ai, Item{Title: "editado", TitleSource: TitleUser}, TitleUser, false},
		{"title change on pending is user", pending, Item{Title: "editado", TitleSource: TitlePending}, TitleUser, false},
		{"same title keeps pending", pending, Item{Title: "borrador", TitleSource: TitlePending}, TitlePending, false},
		{"timeout hands pending to user", pending, Item{Title: "borrador", TitleSource: TitleUser}, TitleUser, false},
		{"same title keeps ai", ai, Item{Title: "Título IA", TitleSource: TitleAI}, TitleAI, false},
		{"pending falls back", pending, Item{Title: "borrador", TitleSource: TitleFallback}, TitleFallback, false},
		{"new fallback rejected", nil, Item{Title: "x", TitleSource: TitleFallback}, "", true},
		{"regeneration reopens fallback", fallback, Item{Title: "borrador", TitleSource: TitlePending}, TitlePending, false},
		{"same title keeps fallback", fallback, Item{Title: "borrador", TitleSource: TitleFallback}, TitleFallback, false},
		{"edit of fallback is user", fallback, Item{Title: "mío", TitleSource: TitleFallback}, TitleUser, false},
		{"ai cannot rewrite fallback", fallback, Item{Title: "otro", TitleSource: TitleAI}, "", true},
		{"unknown source", nil, Item{Title: "x", TitleSource: "bot"}, "", true},
	}
	for _, c := range cases {
		got, err := ResolveTitleSource(c.current, c.in)
		if c.invalid {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("%s: want ErrInvalid, got %v", c.name, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}
