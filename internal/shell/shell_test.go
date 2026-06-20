package shell

import "testing"

func TestExtract(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain text",
			in:   "echo hello",
			want: "echo hello",
		},
		{
			name: "sh fence",
			in:   "```sh\necho hello\n```",
			want: "echo hello",
		},
		{
			name: "bash fence",
			in:   "```bash\necho hello\nls -la\n```",
			want: "echo hello\nls -la",
		},
		{
			name: "untagged fence",
			in:   "```\necho hi\n```",
			want: "echo hi",
		},
		{
			name: "surrounding whitespace",
			in:   "\n\n```sh\necho hi\n```\n\n",
			want: "echo hi",
		},
		{
			name: "fence without closing",
			in:   "```sh\necho hi",
			want: "echo hi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Extract(tt.in); got != tt.want {
				t.Errorf("Extract(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
