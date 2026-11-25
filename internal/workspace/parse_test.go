package workspace

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		prefix  string
		base    string
		want    Names
		wantErr string
	}{
		{
			name:   "happy path",
			arg:    "SOM-123/add-retry-logic",
			prefix: "tscolari",
			base:   "/home/u/worktrees",
			want: Names{
				Ticket:      "SOM-123",
				Description: "add-retry-logic",
				Workspace:   "SOM-123-add-retry-logic",
				Branch:      "tscolari/SOM-123/add-retry-logic",
				Path:        "/home/u/worktrees/SOM-123-add-retry-logic",
				Session:     "SOM-123-add-retry-logic",
			},
		},
		{
			name:   "splits on first slash only",
			arg:    "SOM-123/add/retry/logic",
			prefix: "p",
			base:   "/b",
			want: Names{
				Ticket:      "SOM-123",
				Description: "add/retry/logic",
				Workspace:   "SOM-123-add/retry/logic",
				Branch:      "p/SOM-123/add/retry/logic",
				Path:        "/b/SOM-123-add/retry/logic",
				Session:     "SOM-123-add/retry/logic",
			},
		},
		{
			name:   "trims surrounding whitespace",
			arg:    "  SOM-123/add  ",
			prefix: "p",
			base:   "/b",
			want: Names{
				Ticket:      "SOM-123",
				Description: "add",
				Workspace:   "SOM-123-add",
				Branch:      "p/SOM-123/add",
				Path:        "/b/SOM-123-add",
				Session:     "SOM-123-add",
			},
		},
		{
			name:    "no slash",
			arg:     "SOM-123",
			prefix:  "p",
			base:    "/b",
			wantErr: "TICKET/description",
		},
		{
			name:    "empty arg",
			arg:     "",
			prefix:  "p",
			base:    "/b",
			wantErr: "empty argument",
		},
		{
			name:    "empty ticket",
			arg:     "/desc",
			prefix:  "p",
			base:    "/b",
			wantErr: "ticket part is empty",
		},
		{
			name:    "empty description",
			arg:     "TICK/",
			prefix:  "p",
			base:    "/b",
			wantErr: "description part is empty",
		},
		{
			name:    "empty prefix",
			arg:     "T/d",
			prefix:  "",
			base:    "/b",
			wantErr: "empty branch prefix",
		},
		{
			name:    "empty base",
			arg:     "T/d",
			prefix:  "p",
			base:    "",
			wantErr: "empty worktree base",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.arg, tt.prefix, tt.base)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
