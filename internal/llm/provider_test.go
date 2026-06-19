package llm

import (
	"context"
	"testing"
)

type fakeProvider struct{ name string }

func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) Generate(context.Context, Request) (Response, error) {
	return Response{Text: "ok"}, nil
}

func TestNewReturnsRegisteredProvider(t *testing.T) {
	Register("fake", func(Config) (Provider, error) {
		return fakeProvider{name: "fake"}, nil
	})

	p, err := New("fake", Config{})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if p.Name() != "fake" {
		t.Fatalf("got provider name %q, want %q", p.Name(), "fake")
	}
}

func TestNewUnknownProvider(t *testing.T) {
	if _, err := New("does-not-exist", Config{}); err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

func TestAvailableIsSorted(t *testing.T) {
	Register("zebra", func(Config) (Provider, error) { return fakeProvider{name: "zebra"}, nil })
	Register("alpha", func(Config) (Provider, error) { return fakeProvider{name: "alpha"}, nil })

	names := Available()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("Available() not sorted: %v", names)
		}
	}
}
