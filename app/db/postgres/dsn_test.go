package postgres

import "testing"

func TestEffectiveDSN(t *testing.T) {
	tests := []struct {
		name string
		pg   Postgres
		want string
	}{
		{
			name: "explicit dsn wins",
			pg: Postgres{
				DSN:      "postgres://u:p@db:5432/other?sslmode=require",
				Address:  "ignored:5432",
				User:     "ignored",
				Password: "ignored",
				Database: "ignored",
			},
			want: "postgres://u:p@db:5432/other?sslmode=require",
		},
		{
			name: "discrete fields",
			pg: Postgres{
				Address:  "db:5432",
				User:     "samus",
				Password: "secret",
				Database: "samus",
			},
			want: "postgres://samus:secret@db:5432/samus",
		},
		{
			name: "special chars in password are escaped",
			pg: Postgres{
				Address:  "db:5432",
				User:     "samus",
				Password: "p@ss/w:rd%",
				Database: "samus",
			},
			want: "postgres://samus:p%40ss%2Fw%3Ard%25@db:5432/samus",
		},
		{
			name: "bare host gets default port and database",
			pg: Postgres{
				Address: "db.internal",
				User:    "samus",
			},
			want: "postgres://samus@db.internal:5432/samus",
		},
		{
			name: "empty config falls back to local default",
			pg:   Postgres{},
			want: defaultDSN,
		},
		{
			name: "ssl_mode appended when set",
			pg: Postgres{
				Address:  "db:5432",
				User:     "samus",
				Password: "secret",
				Database: "samus",
				SSLMode:  "require",
			},
			want: "postgres://samus:secret@db:5432/samus?sslmode=require",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pg.EffectiveDSN(); got != tt.want {
				t.Errorf("EffectiveDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}
