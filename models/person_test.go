package models

import "testing"

func TestPersonKindString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind PersonKind
		want string
	}{
		{PersonKindActor, "Actor"},
		{PersonKindDirector, "Director"},
		{PersonKindWriter, "Writer"},
		{PersonKindProducer, "Producer"},
		{PersonKindGuestStar, "GuestStar"},
		{PersonKindComposer, "Composer"},
		{PersonKind(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Fatalf("%v = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestPersonKindFromJob(t *testing.T) {
	t.Parallel()
	cases := map[string]PersonKind{
		"Director": PersonKindDirector, "writer": PersonKindWriter, "Screenplay": PersonKindWriter,
		"Composer": PersonKindComposer, "Music": PersonKindComposer, "Producer": PersonKindProducer,
	}
	for job, want := range cases {
		if got := PersonKindFromJob(job); got != want {
			t.Fatalf("PersonKindFromJob(%q)=%v want %v", job, got, want)
		}
	}
}
